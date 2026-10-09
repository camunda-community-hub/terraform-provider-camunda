package provider

import (
	"context"
	"errors"
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
	f.mux.HandleFunc("POST /members/{email}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	f.mux.HandleFunc("DELETE /members/{email}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return &fetches
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
	if len(invited.Roles) != 1 || invited.Roles[0] != console.ORGANIZATIONROLE_ADMIN {
		t.Errorf("invited roles = %v, want [admin]", invited.Roles)
	}

	if err := client.SetMemberRoles(ctx, "a@example.com", []string{"visitor"}); err != nil {
		t.Fatalf("SetMemberRoles: %v", err)
	}
	updated, err := client.GetMember(ctx, "a@example.com")
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if len(updated.Roles) != 1 || updated.Roles[0] != console.ORGANIZATIONROLE_VISITOR {
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
