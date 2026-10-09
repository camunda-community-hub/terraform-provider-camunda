package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

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
