// Package provider: retry policy and backoff strategy shared across provider
// implementations. Extracted from provider/openai so other backends (Anthropic,
// MiniMax, local) reuse the same well-tested retry behaviour.
package provider

import (
	"context"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// BackoffStrategy defines how the delay between retries grows.
type BackoffStrategy struct {
	// Base is the initial delay before the first retry.
	Base time.Duration
	// Max is the ceiling — delay never exceeds this.
	Max time.Duration
	// Multiplier is the exponential growth factor (typically 2.0).
	Multiplier float64
	// Jitter, when true, applies full jitter: sleep = random(0, computed).
	// When false, jitter is added: sleep = computed + random(0, jitterCap).
	Jitter bool
	// JitterCap is the max additive jitter when Jitter is false.
	JitterCap time.Duration
}

// DefaultBackoff returns a safe, conservative backoff: 500ms→1s→2s→4s→8s(cap).
func DefaultBackoff() BackoffStrategy {
	return BackoffStrategy{
		Base:       500 * time.Millisecond,
		Max:        8 * time.Second,
		Multiplier: 2.0,
		Jitter:     true,
	}
}

// RateLimitBackoff is tuned for 429 responses: longer base, higher cap.
func RateLimitBackoff() BackoffStrategy {
	return BackoffStrategy{
		Base:       5 * time.Second,
		Max:        60 * time.Second,
		Multiplier: 2.0,
		Jitter:     true,
	}
}

// Duration returns the delay for a given attempt number (0-based: attempt 0 =
// first retry after initial failure).
func (b BackoffStrategy) Duration(attempt int) time.Duration {
	computed := float64(b.Base) * math.Pow(b.Multiplier, float64(attempt))
	if computed > float64(b.Max) {
		computed = float64(b.Max)
	}
	d := time.Duration(computed)
	if b.Jitter {
		// Full jitter: sleep = random between 0 and computed.
		if d > 0 {
			d = time.Duration(rand.Int63n(int64(d)))
		}
	} else if b.JitterCap > 0 {
		d += time.Duration(rand.Int63n(int64(b.JitterCap)))
	}
	return d
}

// Sleep blocks for the backoff duration or until ctx is done.
func (b BackoffStrategy) Sleep(ctx context.Context, attempt int) error {
	d := b.Duration(attempt)
	if d <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// RetryPolicy defines when and how to retry an operation.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts including the first one.
	// After MaxAttempts failures, the last error is returned.
	MaxAttempts int
	// Backoff is the delay strategy between attempts.
	Backoff BackoffStrategy
	// RetryAfterHeader, when non-nil, carries a Retry-After duration parsed
	// from the most recent HTTP response. If set, it overrides Backoff for
	// that attempt.
	RetryAfterHeader *time.Duration
}

// DefaultRetryPolicy returns a policy suitable for most transient failures
// (network errors, 408, 5xx): 3 attempts total, default backoff.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		Backoff:     DefaultBackoff(),
	}
}

// RateLimitRetryPolicy returns a policy for 429 responses: 5 attempts, longer backoff.
func RateLimitRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 5,
		Backoff:     RateLimitBackoff(),
	}
}

// ParseRetryAfter reads the Retry-After header from an HTTP response and
// returns the parsed duration. Returns 0 when the header is absent, malformed,
// or exceeds the cap. The cap prevents a malicious/confused server from
// stalling the client indefinitely (120s is generous enough for any real
// rate-limit window).
func ParseRetryAfter(resp *http.Response, cap time.Duration) time.Duration {
	if resp == nil {
		return 0
	}
	ra := resp.Header.Get("Retry-After")
	if ra == "" {
		return 0
	}
	// Try seconds-as-integer first (spec-compliant form).
	if sec, err := strconv.Atoi(ra); err == nil && sec > 0 {
		d := time.Duration(sec) * time.Second
		if d > cap {
			return cap
		}
		return d
	}
	// Try HTTP-date form (less common, but valid per RFC 7231).
	if t, err := time.Parse(time.RFC1123, ra); err == nil {
		d := time.Until(t)
		if d <= 0 || d > cap {
			return 0
		}
		return d
	}
	return 0
}

// IsRetryableStatus reports whether an HTTP status code warrants a retry.
func IsRetryableStatus(code int) bool {
	return code == http.StatusRequestTimeout ||
		code == http.StatusTooManyRequests ||
		(code >= 500 && code <= 599)
}

// IsTransientNetErr reports whether a non-HTTP error (DNS, connection reset,
// EOF) is worth retrying. Context cancellation and deadline expiry are NOT
// transient — they are caller intent and must propagate immediately.
func IsTransientNetErr(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	return true
}
