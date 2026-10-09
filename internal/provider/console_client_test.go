package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	console "github.com/camunda-community-hub/console-customer-api-go"
)

// fakeConsole serves the OAuth token endpoint and lets each test register the
// Console API routes it needs.
type fakeConsole struct {
	*httptest.Server
	mux          *http.ServeMux
	tokensIssued atomic.Int32
}

func newFakeConsole(t *testing.T) *fakeConsole {
	t.Helper()

	f := &fakeConsole{mux: http.NewServeMux()}
	f.mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_, secret, ok := r.BasicAuth()
		if !ok {
			secret = r.FormValue("client_secret")
		}
		if secret != "secret" {
			http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
			return
		}
		n := f.tokensIssued.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// expires_in below oauth2's expiry delta forces a refresh on every use.
		fmt.Fprintf(w, `{"access_token":"token-%d","token_type":"bearer","expires_in":1}`, n)
	})
	f.Server = httptest.NewServer(f.mux)
	t.Cleanup(f.Close)

	return f
}

func (f *fakeConsole) client(t *testing.T) *consoleClient {
	t.Helper()

	client, err := newConsoleClient(context.Background(), consoleClientConfig{
		APIURL:       f.URL,
		TokenURL:     f.URL + "/oauth/token",
		Audience:     "api.cloud.camunda.io",
		ClientID:     "id",
		ClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("newConsoleClient: %v", err)
	}
	client.clusterWaitTimeout = 5 * time.Second
	client.clusterWaitDelay = 0
	client.clusterPollInterval = time.Millisecond

	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func clusterWithStatus(id string, status console.ClusterComponentStatus) console.Cluster {
	return console.Cluster{
		Uuid:   id,
		Name:   "test",
		Status: console.ClusterStatus{Ready: status},
	}
}

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

func TestConsoleClientDeleteTreatsNotFoundAsSuccess(t *testing.T) {
	f := newFakeConsole(t)
	f.mux.HandleFunc("DELETE /clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusNotFound)
	})

	if err := f.client(t).DeleteCluster(context.Background(), "c1"); err != nil {
		t.Fatalf("expected nil, got %v", err)
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
