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
