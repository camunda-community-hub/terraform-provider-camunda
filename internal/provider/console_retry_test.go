package provider

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func response(status int, header http.Header, body string) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

// recordingTransport answers with the given statuses in order and records what it saw.
func recordingTransport(statuses []int, header http.Header) (*rateLimitTransport, *[]string, *[]time.Duration) {
	var bodies []string
	var sleeps []time.Duration

	attempt := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := ""
		if req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			body = string(b)
		}
		bodies = append(bodies, body)

		status := statuses[min(attempt, len(statuses)-1)]
		attempt++
		return response(status, header, "status "+http.StatusText(status)), nil
	})

	tr := newRateLimitTransport(base)
	tr.sleep = func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return ctx.Err()
	}
	return tr, &bodies, &sleeps
}

// newPost builds a replayable POST, as made through withReplayableWrite.
func newPost(t *testing.T, ctx context.Context) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(withReplayableWrite(ctx), http.MethodPost, "http://console.test/members/a@b.c", strings.NewReader(`{"roles":["admin"]}`))
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestRateLimitTransportRetriesUntilSuccess(t *testing.T) {
	tr, bodies, sleeps := recordingTransport([]int{429, 429, 200}, nil)

	resp, err := tr.RoundTrip(newPost(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if len(*bodies) != 3 {
		t.Fatalf("attempts = %d, want 3", len(*bodies))
	}
	for i, body := range *bodies {
		if body != `{"roles":["admin"]}` {
			t.Errorf("attempt %d sent body %q, want the original body", i, body)
		}
	}
	if len(*sleeps) != 2 {
		t.Fatalf("slept %d times, want 2", len(*sleeps))
	}
}

func TestRateLimitTransportBackoffGrowsWithJitterAndIsCapped(t *testing.T) {
	tr, _, sleeps := recordingTransport([]int{429}, nil)
	tr.maxRetries = 8
	tr.maxWait = time.Hour

	resp, err := tr.RoundTrip(newPost(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if len(*sleeps) != 8 {
		t.Fatalf("slept %d times, want 8", len(*sleeps))
	}
	for attempt, got := range *sleeps {
		full := min(tr.minBackoff<<attempt, tr.maxBackoff)
		if got < full/2 || got > full {
			t.Errorf("attempt %d slept %s, want between %s and %s", attempt, got, full/2, full)
		}
	}
}

func TestRateLimitTransportHonorsRetryAfter(t *testing.T) {
	tr, _, sleeps := recordingTransport([]int{429, 200}, http.Header{"Retry-After": []string{"3"}})

	if _, err := tr.RoundTrip(newPost(t, context.Background())); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	if len(*sleeps) != 1 || (*sleeps)[0] != 3*time.Second {
		t.Errorf("slept %v, want [3s]", *sleeps)
	}
}

func TestRateLimitTransportCapsRetryAfter(t *testing.T) {
	tr, _, sleeps := recordingTransport([]int{429, 200}, http.Header{"Retry-After": []string{"3600"}})

	if _, err := tr.RoundTrip(newPost(t, context.Background())); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	if len(*sleeps) != 1 || (*sleeps)[0] != tr.maxBackoff {
		t.Errorf("slept %v, want [%s]", *sleeps, tr.maxBackoff)
	}
}

func TestRateLimitTransportGivesUpWithLastResponse(t *testing.T) {
	tr, bodies, _ := recordingTransport([]int{429}, nil)
	tr.maxRetries = 2

	resp, err := tr.RoundTrip(newPost(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", resp.StatusCode)
	}
	if got, _ := io.ReadAll(resp.Body); !strings.Contains(string(got), "Too Many Requests") {
		t.Errorf("final response body was consumed or replaced: %q", got)
	}
	if len(*bodies) != 3 {
		t.Errorf("attempts = %d, want 1 try + 2 retries", len(*bodies))
	}
}

func TestRateLimitTransportDoesNotRetryOtherStatuses(t *testing.T) {
	tr, bodies, _ := recordingTransport([]int{500}, nil)

	if _, err := tr.RoundTrip(newPost(t, context.Background())); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	if len(*bodies) != 1 {
		t.Errorf("attempts = %d, want 1", len(*bodies))
	}
}

func TestRateLimitTransportStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tr, bodies, _ := recordingTransport([]int{429}, nil)
	tr.sleep = func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := tr.RoundTrip(newPost(t, ctx))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(*bodies) != 1 {
		t.Errorf("attempts = %d, want 1", len(*bodies))
	}
}

func TestRateLimitTransportDoesNotRetryBodiesItCannotReplay(t *testing.T) {
	tr, bodies, _ := recordingTransport([]int{429}, nil)

	req := newPost(t, context.Background())
	req.GetBody = nil

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusTooManyRequests || len(*bodies) != 1 {
		t.Errorf("status = %d, attempts = %d, want the first 429 returned", resp.StatusCode, len(*bodies))
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		value string
		want  time.Duration
		ok    bool
	}{
		"empty":     {"", 0, false},
		"seconds":   {"7", 7 * time.Second, true},
		"zero":      {"0", 0, true},
		"negative":  {"-1", 0, false},
		"http date": {now.Add(10 * time.Second).Format(http.TimeFormat), 10 * time.Second, true},
		"past date": {now.Add(-time.Minute).Format(http.TimeFormat), 0, true},
		"garbage":   {"soon", 0, false},
		"overflows when converted to nanoseconds":  {"9223372037", time.Duration(math.MaxInt64), true},
		"larger than int64":                        {"99999999999999999999", 0, false},
		"largest value that still fits a duration": {"9223372036", 9223372036 * time.Second, true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := retryAfter(tc.value, now)
			if got != tc.want || ok != tc.ok {
				t.Errorf("retryAfter(%q) = %s, %v; want %s, %v", tc.value, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestRateLimitTransportDoesNotReplayUnsafeWrites(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			tr, bodies, _ := recordingTransport([]int{429, 200}, nil)

			req, err := http.NewRequest(method, "http://console.test/clusters", strings.NewReader(`{"name":"c"}`))
			if err != nil {
				t.Fatal(err)
			}

			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusTooManyRequests || len(*bodies) != 1 {
				t.Errorf("status = %d, attempts = %d, want the first 429 returned without a retry", resp.StatusCode, len(*bodies))
			}
		})
	}
}

func TestRateLimitTransportReplaysIdempotentMethods(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			tr, bodies, _ := recordingTransport([]int{429, 200}, nil)

			req, err := http.NewRequest(method, "http://console.test/clusters/c1", nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK || len(*bodies) != 2 {
				t.Errorf("status = %d, attempts = %d, want a retry that succeeds", resp.StatusCode, len(*bodies))
			}
		})
	}
}

func TestRateLimitTransportStopsWhenTotalWaitWouldExceedBudget(t *testing.T) {
	tr, bodies, sleeps := recordingTransport([]int{429}, http.Header{"Retry-After": []string{"30"}})
	tr.maxWait = 70 * time.Second

	resp, err := tr.RoundTrip(newPost(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want the last 429", resp.StatusCode)
	}
	// Two 30s waits fit into 70s, a third would not.
	if len(*sleeps) != 2 || len(*bodies) != 3 {
		t.Errorf("slept %d times over %d attempts, want 2 waits and 3 attempts", len(*sleeps), len(*bodies))
	}
}

func TestRateLimitTransportDefaultsStayWithinTheWaitBudget(t *testing.T) {
	tr, _, sleeps := recordingTransport([]int{429}, http.Header{"Retry-After": []string{"30"}})

	resp, err := tr.RoundTrip(newPost(t, context.Background()))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var total time.Duration
	for _, d := range *sleeps {
		total += d
	}
	if total > tr.maxWait {
		t.Errorf("waited %s in total, want at most %s", total, tr.maxWait)
	}
}

func TestRateLimitTransportCapsHugeRetryAfter(t *testing.T) {
	tr, _, sleeps := recordingTransport([]int{429, 200}, http.Header{"Retry-After": []string{"9223372037"}})

	if _, err := tr.RoundTrip(newPost(t, context.Background())); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	if len(*sleeps) != 1 || (*sleeps)[0] != tr.maxBackoff {
		t.Errorf("slept %v, want [%s] rather than an immediate retry", *sleeps, tr.maxBackoff)
	}
}
