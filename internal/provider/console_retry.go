package provider

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	defaultRateLimitRetries    = 8
	defaultRateLimitMinBackoff = 500 * time.Millisecond
	defaultRateLimitMaxBackoff = 30 * time.Second
	defaultRateLimitMaxWait    = 60 * time.Second
)

// replayableWriteKey marks a context whose POST or PATCH request may be
// replayed after a 429.
type replayableWriteKey struct{}

// withReplayableWrite marks a POST or PATCH call as safe to retry. Only use it
// for calls that set state, so applying them twice is the same as applying
// them once; never for calls that create objects.
func withReplayableWrite(ctx context.Context) context.Context {
	return context.WithValue(ctx, replayableWriteKey{}, true)
}

// canReplay reports whether a request can be sent again without risking a
// duplicate side effect. A 429 alone does not prove that the server did not
// process the request, so non-idempotent methods are only replayed when the
// caller opted in.
func canReplay(req *http.Request) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace, http.MethodPut, http.MethodDelete:
		return true
	}
	return req.Context().Value(replayableWriteKey{}) != nil
}

// rateLimitTransport retries requests that the Console API rejects with
// 429 Too Many Requests, as long as they are safe to replay (see canReplay).
//
// It waits for the Retry-After header when the server sends one and otherwise
// backs off exponentially with jitter. After maxRetries retries, or once the
// next wait would exceed maxWait in total, it returns the last 429 response
// unchanged, so the caller reports the server's own message.
type rateLimitTransport struct {
	base       http.RoundTripper
	maxRetries int
	minBackoff time.Duration
	maxBackoff time.Duration

	// maxWait bounds the total time spent waiting between attempts for one
	// request, whatever the other settings add up to.
	maxWait time.Duration

	// Overridden in tests.
	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time
}

func newRateLimitTransport(base http.RoundTripper) *rateLimitTransport {
	return &rateLimitTransport{
		base:       base,
		maxRetries: defaultRateLimitRetries,
		minBackoff: defaultRateLimitMinBackoff,
		maxBackoff: defaultRateLimitMaxBackoff,
		maxWait:    defaultRateLimitMaxWait,
		sleep:      sleepContext,
		now:        time.Now,
	}
}

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !canReplay(req) {
		return t.base.RoundTrip(req)
	}

	var waited time.Duration

	for attempt := 0; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt >= t.maxRetries {
			return resp, err
		}

		// A request body can only be replayed if it can be recreated.
		if !canRewind(req) {
			return resp, nil
		}

		delay := t.delay(resp, attempt)
		if waited+delay > t.maxWait {
			return resp, nil
		}
		waited += delay

		tflog.Debug(req.Context(), "Console API rate limited the request, retrying", map[string]interface{}{
			"method":  req.Method,
			"path":    req.URL.Path,
			"attempt": attempt + 1,
			"delay":   delay.String(),
		})

		// Release the connection before waiting.
		_ = resp.Body.Close()

		if err := t.sleep(req.Context(), delay); err != nil {
			return nil, fmt.Errorf("giving up on rate limited request: %w", err)
		}

		// Recreate the body only now that the request is really going out again,
		// so no early return leaves an unsent body open.
		next, err := rewind(req)
		if err != nil {
			return nil, fmt.Errorf("unable to replay rate limited request: %w", err)
		}
		req = next
	}
}

// canRewind reports whether rewind can recreate the request body.
func canRewind(req *http.Request) bool {
	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}

// rewind returns a copy of req with a fresh body, so it can be sent again.
func rewind(req *http.Request) (*http.Request, error) {
	next := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return next, nil
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	next.Body = body
	return next, nil
}

// delay is how long to wait before retry number attempt+1.
func (t *rateLimitTransport) delay(resp *http.Response, attempt int) time.Duration {
	if d, ok := retryAfter(resp.Header.Get("Retry-After"), t.now()); ok {
		return min(d, t.maxBackoff)
	}

	backoff := t.minBackoff << min(attempt, 30)
	if backoff <= 0 || backoff > t.maxBackoff {
		backoff = t.maxBackoff
	}
	// Spread concurrent requests: wait between half and the full backoff.
	return backoff/2 + rand.N(backoff/2+1)
}

// retryAfter parses a Retry-After header, given either in seconds or as an
// HTTP date.
func retryAfter(value string, now time.Time) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		// Saturate instead of overflowing time.Duration into a negative wait.
		if seconds > int64(math.MaxInt64/time.Second) {
			return time.Duration(math.MaxInt64), true
		}
		return time.Duration(seconds) * time.Second, true
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(at.Sub(now), 0), true
	}
	return 0, false
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
