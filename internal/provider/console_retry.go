package provider

import (
	"context"
	"fmt"
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
)

// rateLimitTransport retries requests that the Console API rejects with
// 429 Too Many Requests. A rate limited request was not processed, so
// replaying it is safe for every method.
//
// It waits for the Retry-After header when the server sends one and otherwise
// backs off exponentially with jitter. After maxRetries retries it returns the
// last 429 response unchanged, so the caller reports the server's own message.
type rateLimitTransport struct {
	base       http.RoundTripper
	maxRetries int
	minBackoff time.Duration
	maxBackoff time.Duration

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
		sleep:      sleepContext,
		now:        time.Now,
	}
}

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt >= t.maxRetries {
			return resp, err
		}

		// A request body can only be replayed if it can be recreated.
		next, ok := rewind(req)
		if !ok {
			return resp, nil
		}

		delay := t.delay(resp, attempt)
		tflog.Debug(req.Context(), "Console API rate limited the request, retrying", map[string]interface{}{
			"method":  req.Method,
			"path":    req.URL.Path,
			"attempt": attempt + 1,
			"delay":   delay.String(),
		})

		// Release the connection before waiting.
		resp.Body.Close()

		if err := t.sleep(req.Context(), delay); err != nil {
			return nil, fmt.Errorf("giving up on rate limited request: %w", err)
		}
		req = next
	}
}

// rewind returns a copy of req that can be sent again.
func rewind(req *http.Request) (*http.Request, bool) {
	next := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return next, true
	}
	if req.GetBody == nil {
		return nil, false
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, false
	}
	next.Body = body
	return next, true
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
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
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
