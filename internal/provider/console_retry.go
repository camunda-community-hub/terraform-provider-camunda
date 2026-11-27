package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	defaultRateLimitRetries    = 8
	defaultRateLimitMinBackoff = 500 * time.Millisecond
	defaultRateLimitMaxBackoff = 30 * time.Second
	defaultRateLimitMaxWait    = 60 * time.Second

	// maxDrainBytes is how much of a discarded retried body is read to keep the
	// connection reusable.
	maxDrainBytes = 4096
)

// replayableWriteKey marks a context whose POST or PATCH request may be
// replayed after a 429, 502, 503 or 504.
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

// isRetryableStatus reports whether a response status is transient: the
// Console API is rate limiting or briefly unavailable.
func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// rateLimitTransport retries requests that the Console API answers with
// 429 Too Many Requests, 502 Bad Gateway, 503 Service Unavailable or
// 504 Gateway Timeout, as long as they are safe to replay (see canReplay).
// It also retries the 409 Conflict for a connector secret change that is
// already in progress (see isSecretsChangeInProgress), for every method.
//
// It waits for the Retry-After header when the server sends one and otherwise
// backs off exponentially with jitter. After maxRetries retries, or once the
// next wait would exceed maxWait in total, it returns the last response
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

// isSecretsChangeInProgress reports whether resp is the 409 Conflict the
// Console API sends when another change to the connector secrets of the same
// cluster is still running. Such a request was rejected without any effect, so
// it is safe to replay whatever its method. A 409 for another reason, such as
// a secret that already exists, does not match.
//
// The start of the body is read to tell the cases apart and put back, so the
// caller can still read the whole body.
func isSecretsChangeInProgress(resp *http.Response) bool {
	if resp.StatusCode != http.StatusConflict || resp.Body == nil {
		return false
	}

	head, _ := io.ReadAll(io.LimitReader(resp.Body, maxDrainBytes))
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(head), resp.Body), resp.Body}

	message := strings.ToLower(string(head))
	return strings.Contains(message, "another change") && strings.Contains(message, "in progress")
}

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	replayable := canReplay(req)

	var waited time.Duration

	for attempt := 0; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if err != nil || attempt >= t.maxRetries {
			return resp, err
		}
		transient := replayable && isRetryableStatus(resp.StatusCode)
		if !transient && !isSecretsChangeInProgress(resp) {
			return resp, nil
		}

		// A request body can only be replayed if it can be recreated.
		if !canRewind(req) {
			return resp, nil
		}

		delay := t.delay(resp, attempt)
		// Written as a subtraction so a saturated Retry-After cannot overflow.
		if delay > t.maxWait-waited {
			return resp, nil
		}
		waited += delay

		tflog.Debug(req.Context(), "Console API answered with a transient error, retrying", map[string]interface{}{
			"method":  req.Method,
			"status":  resp.StatusCode,
			"path":    req.URL.Path,
			"attempt": attempt + 1,
			"delay":   delay.String(),
		})

		// Drain a little of the body so the connection can be reused, then
		// release it before waiting.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
		_ = resp.Body.Close()

		if err := t.sleep(req.Context(), delay); err != nil {
			return nil, fmt.Errorf("giving up on retried request: %w", err)
		}

		// Recreate the body only now that the request is really going out again,
		// so no early return leaves an unsent body open.
		next, err := rewind(req)
		if err != nil {
			return nil, fmt.Errorf("unable to replay request: %w", err)
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
	// Retrying before the time the server asked for only earns another 429, so
	// Retry-After is never shortened. If it does not fit into the wait budget,
	// RoundTrip gives up instead.
	// An already elapsed Retry-After still waits minBackoff, so the retries
	// never run back to back.
	if d, ok := retryAfter(resp.Header.Get("Retry-After"), t.now()); ok {
		return max(d, t.minBackoff)
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
	if isDigits(value) {
		// Every all-digit value is valid delta-seconds, however large. Saturate
		// instead of overflowing time.Duration into a negative or early wait.
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil || seconds > int64(math.MaxInt64/time.Second) {
			return time.Duration(math.MaxInt64), true
		}
		return time.Duration(seconds) * time.Second, true
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(at.Sub(now), 0), true
	}
	return 0, false
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
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
