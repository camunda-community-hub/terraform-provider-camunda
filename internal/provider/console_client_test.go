package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	console "github.com/camunda-community-hub/console-customer-api-go"
)

func TestConsoleClientRejectsInvalidCredentials(t *testing.T) {
	f := newFakeConsole(t)

	_, err := newConsoleClient(context.Background(), consoleClientConfig{
		APIURL:       f.URL,
		TokenURL:     f.URL + "/oauth/token",
		ClientID:     "id",
		ClientSecret: "wrong",
	})
	if err == nil {
		t.Fatal("expected an error for invalid credentials")
	}
}

func TestConsoleClientSendsFreshTokens(t *testing.T) {
	f := newFakeConsole(t)
	var seen []string
	f.mux.HandleFunc("GET /clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		writeJSON(t, w, clusterWithStatus(r.PathValue("id"), console.CLUSTERCOMPONENTSTATUS_HEALTHY))
	})
	client := f.client(t)

	for range 2 {
		if _, err := client.GetCluster(context.Background(), "c1"); err != nil {
			t.Fatalf("GetCluster: %v", err)
		}
	}

	if len(seen) != 2 || seen[0] == seen[1] || !strings.HasPrefix(seen[0], "Bearer token-") {
		t.Fatalf("expected two distinct bearer tokens, got %q", seen)
	}
}

func TestConsoleClientGetReportsNotFound(t *testing.T) {
	f := newFakeConsole(t)
	f.mux.HandleFunc("GET /clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"cluster gone"}`, http.StatusNotFound)
	})

	_, err := f.client(t).GetCluster(context.Background(), "c1")

	if !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "cluster gone") {
		t.Fatalf("expected the response body in the error, got %q", err)
	}
}

func TestConsoleClientDeleteReportsNotFound(t *testing.T) {
	f := newFakeConsole(t)
	f.mux.HandleFunc("DELETE /clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusNotFound)
	})

	if err := f.client(t).DeleteCluster(context.Background(), "c1"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

func TestConsoleClientDeleteReportsServerErrors(t *testing.T) {
	f := newFakeConsole(t)
	f.mux.HandleFunc("DELETE /clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})

	err := f.client(t).DeleteCluster(context.Background(), "c1")

	if err == nil || errors.Is(err, errNotFound) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected a server error carrying the body, got %v", err)
	}
}

func TestConsoleClientWaitClusterHealthy(t *testing.T) {
	f := newFakeConsole(t)
	statuses := []console.ClusterComponentStatus{
		console.CLUSTERCOMPONENTSTATUS_CREATING,
		console.CLUSTERCOMPONENTSTATUS_HEALTHY,
		console.CLUSTERCOMPONENTSTATUS_HEALTHY,
	}
	var calls atomic.Int32
	f.mux.HandleFunc("GET /clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		i := min(int(calls.Add(1))-1, len(statuses)-1)
		writeJSON(t, w, clusterWithStatus(r.PathValue("id"), statuses[i]))
	})

	if err := f.client(t).WaitClusterHealthy(context.Background(), "c1"); err != nil {
		t.Fatalf("WaitClusterHealthy: %v", err)
	}
	if calls.Load() < 3 {
		t.Fatalf("expected to poll until healthy twice, polled %d times", calls.Load())
	}
}

func TestConsoleClientGetSecretReportsMissingName(t *testing.T) {
	f := newFakeConsole(t)
	f.mux.HandleFunc("GET /clusters/{id}/secrets", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]string{"present": "value"})
	})
	client := f.client(t)

	value, err := client.GetSecret(context.Background(), "c1", "present")
	if err != nil || value != "value" {
		t.Fatalf("GetSecret(present) = %q, %v", value, err)
	}

	if _, err := client.GetSecret(context.Background(), "c1", "missing"); !errors.Is(err, errNotFound) {
		t.Fatalf("expected errNotFound, got %v", err)
	}
}

func TestConsoleClientClearsIPAllowlist(t *testing.T) {
	f := newFakeConsole(t)
	var body string
	f.mux.HandleFunc("PUT /clusters/{id}/ipallowlist", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusNoContent)
	})

	if err := f.client(t).SetIPAllowlist(context.Background(), "c1", nil); err != nil {
		t.Fatalf("SetIPAllowlist: %v", err)
	}
	if !strings.Contains(body, `"ipallowlist":[]`) {
		t.Fatalf("expected an empty allowlist in the request, got %s", body)
	}
}

// memberServer serves a fixed member list and counts how often it is fetched.
func memberServer(t *testing.T, f *fakeConsole, emails ...string) *atomic.Int32 {
	t.Helper()

	var fetches atomic.Int32
	f.mux.HandleFunc("GET /members", func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		members := []console.Member{}
		for _, email := range emails {
			members = append(members, console.Member{Email: email, Roles: []console.OrganizationRole{console.ORGANIZATIONROLE_DEVELOPER}})
		}
		writeJSON(t, w, members)
	})
	return &fetches
}

// memberWrites accepts every member update and delete.
func memberWrites(f *fakeConsole) {
	f.mux.HandleFunc("POST /members/{email}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	f.mux.HandleFunc("DELETE /members/{email}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestConsoleClientFetchesMemberListOnceForConcurrentLookups(t *testing.T) {
	f := newFakeConsole(t)
	fetches := memberServer(t, f, "a@example.com", "b@example.com")
	client := f.client(t)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.GetMember(context.Background(), "a@example.com"); err != nil {
				t.Errorf("GetMember: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := fetches.Load(); got != 1 {
		t.Errorf("fetched the member list %d times, want 1", got)
	}
	if _, err := client.GetMember(context.Background(), "missing@example.com"); !errors.Is(err, errNotFound) {
		t.Errorf("err = %v, want errNotFound", err)
	}
	if got := fetches.Load(); got != 1 {
		t.Errorf("a miss refetched the member list (%d fetches)", got)
	}
}

func TestConsoleClientRefetchesMemberListAfterTTL(t *testing.T) {
	f := newFakeConsole(t)
	fetches := memberServer(t, f, "a@example.com")
	client := f.client(t)

	now := time.Now()
	client.members.now = func() time.Time { return now }

	for range 2 {
		if _, err := client.GetMember(context.Background(), "a@example.com"); err != nil {
			t.Fatalf("GetMember: %v", err)
		}
	}
	if got := fetches.Load(); got != 1 {
		t.Fatalf("fetches within the TTL = %d, want 1", got)
	}

	now = now.Add(memberCacheTTL)
	if _, err := client.GetMember(context.Background(), "a@example.com"); err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if got := fetches.Load(); got != 2 {
		t.Errorf("fetches after the TTL = %d, want 2", got)
	}
}

func TestConsoleClientKeepsMemberCacheCurrentAfterWrites(t *testing.T) {
	f := newFakeConsole(t)
	fetches := memberServer(t, f, "a@example.com")
	memberWrites(f)
	client := f.client(t)
	ctx := context.Background()

	if _, err := client.GetMember(ctx, "a@example.com"); err != nil {
		t.Fatalf("GetMember: %v", err)
	}

	if err := client.SetMemberRoles(ctx, "new@example.com", []string{"admin"}); err != nil {
		t.Fatalf("SetMemberRoles: %v", err)
	}
	invited, err := client.GetMember(ctx, "new@example.com")
	if err != nil {
		t.Fatalf("invited member not found: %v", err)
	}
	if len(invited.Roles) != 1 || invited.Roles[0] != "admin" {
		t.Errorf("invited roles = %v, want [admin]", invited.Roles)
	}

	if err := client.SetMemberRoles(ctx, "a@example.com", []string{"visitor"}); err != nil {
		t.Fatalf("SetMemberRoles: %v", err)
	}
	updated, err := client.GetMember(ctx, "a@example.com")
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if len(updated.Roles) != 1 || updated.Roles[0] != "visitor" {
		t.Errorf("updated roles = %v, want [visitor]", updated.Roles)
	}

	if err := client.DeleteMember(ctx, "a@example.com"); err != nil {
		t.Fatalf("DeleteMember: %v", err)
	}
	if _, err := client.GetMember(ctx, "a@example.com"); !errors.Is(err, errNotFound) {
		t.Errorf("deleted member: err = %v, want errNotFound", err)
	}

	if got := fetches.Load(); got != 1 {
		t.Errorf("writes caused %d fetches, want the single initial one", got)
	}
}

func TestConsoleClientRetriesRateLimitedRequests(t *testing.T) {
	f := newFakeConsole(t)
	var calls atomic.Int32
	f.mux.HandleFunc("GET /clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "local_rate_limited", http.StatusTooManyRequests)
			return
		}
		writeJSON(t, w, clusterWithStatus(r.PathValue("id"), console.CLUSTERCOMPONENTSTATUS_HEALTHY))
	})

	if _, err := f.client(t).GetCluster(context.Background(), "c1"); err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestConsoleClientSendsAFreshTokenOnEveryRetry(t *testing.T) {
	f := newFakeConsole(t)
	var seen []string
	f.mux.HandleFunc("GET /clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if len(seen) < 3 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "local_rate_limited", http.StatusTooManyRequests)
			return
		}
		writeJSON(t, w, clusterWithStatus(r.PathValue("id"), console.CLUSTERCOMPONENTSTATUS_HEALTHY))
	})

	if _, err := f.client(t).GetCluster(context.Background(), "c1"); err != nil {
		t.Fatalf("GetCluster: %v", err)
	}

	// The fake issues tokens that expire immediately, so a token attached once
	// outside the retry loop would repeat.
	if len(seen) != 3 || seen[0] == seen[1] || seen[1] == seen[2] {
		t.Errorf("expected a distinct bearer token per attempt, got %q", seen)
	}
}

// rateLimitFirst answers the first call to a route with 429 and counts calls.
func rateLimitFirst(t *testing.T, f *fakeConsole, pattern string, ok func(w http.ResponseWriter, r *http.Request)) (calls *atomic.Int32, bodies *[]string) {
	t.Helper()

	var n atomic.Int32
	var seen []string
	var mu sync.Mutex
	f.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, string(b))
		mu.Unlock()

		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "local_rate_limited", http.StatusTooManyRequests)
			return
		}
		ok(w, r)
	})
	return &n, &seen
}

func TestConsoleClientReplaysOnlyWritesThatSetState(t *testing.T) {
	ctx := context.Background()

	replayed := map[string]struct {
		pattern string
		call    func(c *consoleClient) error
		ok      func(w http.ResponseWriter, r *http.Request)
	}{
		"UpdateCluster (PATCH)": {
			pattern: "PATCH /clusters/{id}",
			call:    func(c *consoleClient) error { return c.UpdateCluster(ctx, "c1", "name", "desc") },
			ok:      func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) },
		},
		"SetMemberRoles (POST)": {
			pattern: "POST /members/{email}",
			call:    func(c *consoleClient) error { return c.SetMemberRoles(ctx, "a@example.com", []string{"admin"}) },
			ok:      func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) },
		},
	}
	for name, tc := range replayed {
		t.Run("replays "+name, func(t *testing.T) {
			f := newFakeConsole(t)
			calls, bodies := rateLimitFirst(t, f, tc.pattern, tc.ok)

			if err := tc.call(f.client(t)); err != nil {
				t.Fatalf("expected the retry to succeed, got %v", err)
			}
			if calls.Load() != 2 {
				t.Fatalf("calls = %d, want 2", calls.Load())
			}
			if len(*bodies) != 2 || (*bodies)[0] == "" || (*bodies)[0] != (*bodies)[1] {
				t.Errorf("the retry must resend the same non-empty body, got %q", *bodies)
			}
		})
	}

	notReplayed := map[string]struct {
		pattern string
		call    func(c *consoleClient) error
	}{
		"CreateCluster": {
			pattern: "POST /clusters",
			call: func(c *consoleClient) error {
				_, err := c.CreateCluster(ctx, cluster{Name: "c"})
				return err
			},
		},
		"CreateClusterClient": {
			pattern: "POST /clusters/{id}/clients",
			call: func(c *consoleClient) error {
				_, err := c.CreateClusterClient(ctx, "c1", "client", []string{"Zeebe"})
				return err
			},
		},
		"CreateSecret": {
			pattern: "POST /clusters/{id}/secrets",
			call:    func(c *consoleClient) error { return c.CreateSecret(ctx, "c1", "name", "value") },
		},
	}
	for name, tc := range notReplayed {
		t.Run("does not replay "+name, func(t *testing.T) {
			f := newFakeConsole(t)
			calls, _ := rateLimitFirst(t, f, tc.pattern, func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("%s must not be sent a second time", name)
				w.WriteHeader(http.StatusOK)
			})

			err := tc.call(f.client(t))

			if err == nil || !strings.Contains(err.Error(), "local_rate_limited") {
				t.Errorf("expected the 429 to be reported, got %v", err)
			}
			if calls.Load() != 1 {
				t.Errorf("calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestConsoleClientEvictsMemberFromCacheWhenDeleteReportsNotFound(t *testing.T) {
	f := newFakeConsole(t)
	fetches := memberServer(t, f, "a@example.com")
	f.mux.HandleFunc("DELETE /members/{email}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	})
	client := f.client(t)
	ctx := context.Background()

	if _, err := client.GetMember(ctx, "a@example.com"); err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if err := client.DeleteMember(ctx, "a@example.com"); !errors.Is(err, errNotFound) {
		t.Fatalf("DeleteMember err = %v, want errNotFound", err)
	}

	if _, err := client.GetMember(ctx, "a@example.com"); !errors.Is(err, errNotFound) {
		t.Errorf("member must be gone from the cache, got %v", err)
	}
	if got := fetches.Load(); got != 1 {
		t.Errorf("fetches = %d, want 1", got)
	}
}

func TestConsoleClientLeavesMemberCacheUntouchedWhenWriteFails(t *testing.T) {
	f := newFakeConsole(t)
	memberServer(t, f, "a@example.com")
	f.mux.HandleFunc("POST /members/{email}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	client := f.client(t)
	ctx := context.Background()

	before, err := client.GetMember(ctx, "a@example.com")
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if err := client.SetMemberRoles(ctx, "a@example.com", []string{"visitor"}); err == nil {
		t.Fatal("expected SetMemberRoles to fail")
	}
	if err := client.SetMemberRoles(ctx, "new@example.com", []string{"visitor"}); err == nil {
		t.Fatal("expected SetMemberRoles to fail")
	}

	after, err := client.GetMember(ctx, "a@example.com")
	if err != nil || len(after.Roles) != len(before.Roles) || after.Roles[0] != before.Roles[0] {
		t.Errorf("failed write changed the cached roles: %v (err %v)", after, err)
	}
	if _, err := client.GetMember(ctx, "new@example.com"); !errors.Is(err, errNotFound) {
		t.Errorf("failed invite must not be cached, got %v", err)
	}
}

func TestConsoleClientMemberCacheSurvivesConcurrentReadsAndWrites(t *testing.T) {
	f := newFakeConsole(t)
	memberServer(t, f, "a@example.com")
	memberWrites(f)
	client := f.client(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = client.GetMember(ctx, "a@example.com")
		}()
		go func() {
			defer wg.Done()
			if err := client.SetMemberRoles(ctx, fmt.Sprintf("m%d@example.com", i), []string{"admin"}); err != nil {
				t.Errorf("SetMemberRoles: %v", err)
			}
		}()
	}
	wg.Wait()

	for i := range 10 {
		if _, err := client.GetMember(ctx, fmt.Sprintf("m%d@example.com", i)); err != nil {
			t.Errorf("member m%d missing after concurrent writes: %v", i, err)
		}
	}
}

func TestConsoleClientReadsTheDeprecatedIPWhitelist(t *testing.T) {
	f := newFakeConsole(t)
	f.mux.HandleFunc("GET /clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		found := clusterWithStatus(r.PathValue("id"), console.CLUSTERCOMPONENTSTATUS_HEALTHY)
		found.Ipwhitelist = []console.ClusterIpallowlistInner{{Ip: "10.0.0.1", Description: "office"}}
		writeJSON(t, w, found)
	})

	entries, err := f.client(t).GetIPAllowlist(context.Background(), "c1")

	if err != nil || len(entries) != 1 || entries[0] != (allowlistEntry{IP: "10.0.0.1", Description: "office"}) {
		t.Fatalf("expected the deprecated allowlist, got %v, %v", entries, err)
	}
}

// serveOwner makes the fake Console list one owner and fail the test on any
// attempt to change members.
func serveOwner(t *testing.T, f *fakeConsole) {
	f.mux.HandleFunc("GET /members", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, []console.Member{{
			Email: "owner@example.com",
			Roles: []console.OrganizationRole{console.ORGANIZATIONROLE_OWNER, "admin", "modeler"},
		}})
	})
	for _, route := range []string{"POST /members/{email}", "DELETE /members/{email}"} {
		f.mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		})
	}
}

func TestConsoleClientGetMemberKeepsOnlyAssignableRoles(t *testing.T) {
	f := newFakeConsole(t)
	serveOwner(t, f)

	found, err := f.client(t).GetMember(context.Background(), "owner@example.com")

	if err != nil || !found.Owner || len(found.Roles) != 1 || found.Roles[0] != "admin" {
		t.Fatalf("expected the owner with only the admin role, got %+v, %v", found, err)
	}
}

func TestConsoleClientRefusesToChangeTheOwner(t *testing.T) {
	f := newFakeConsole(t)
	serveOwner(t, f)
	client := f.client(t)

	if err := client.SetMemberRoles(context.Background(), "owner@example.com", []string{"analyst"}); !errors.Is(err, errOwnerUnchangeable) {
		t.Fatalf("SetMemberRoles: expected errOwnerUnchangeable, got %v", err)
	}
	if err := client.DeleteMember(context.Background(), "owner@example.com"); !errors.Is(err, errOwnerUnchangeable) {
		t.Fatalf("DeleteMember: expected errOwnerUnchangeable, got %v", err)
	}
}
