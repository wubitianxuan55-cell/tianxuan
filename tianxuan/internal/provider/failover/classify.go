// Package failover implements a turn-local model fallback chain distilled
// from OpenClaw's model-failover design: when the primary model fails with a
// failover-worthy error (rate limit, overload, transport break), the chain
// tries the next configured candidate; a fallback applies only to the current
// turn so the next turn returns to the primary and keeps its prompt-cache
// prefix stable.
package failover

import (
	"context"
	"errors"
	"strings"

	"tianxuan/internal/provider"
)

// ShouldFailover reports whether a candidate failure warrants moving to the
// next model in the chain. Non-failover-worthy errors (parameter errors, auth
// failures with the same key, caller cancellation, mid-stream interruption)
// propagate immediately — retrying another model cannot fix them.
func ShouldFailover(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var auth *provider.AuthError
	if errors.As(err, &auth) {
		// Same API key across candidates: switching models cannot fix auth.
		return false
	}
	if provider.IsStreamInterrupted(err) {
		// Mid-stream interruption is handled by the agent's recovery loop.
		return false
	}
	if code := provider.HTTPStatus(err); code != 0 {
		return provider.IsRetryableStatus(code)
	}
	return isNetworkish(err)
}

// IsOverload reports whether a failure is a pure overload condition (rate
// limit, server error, or transport break). Only when every candidate in a
// round fails with overload does the chain retry the whole round with
// exponential backoff.
func IsOverload(err error) bool {
	if err == nil {
		return false
	}
	if code := provider.HTTPStatus(err); code != 0 {
		return code == 429 || (code >= 500 && code <= 599)
	}
	return isNetworkish(err)
}

var networkMarkers = []string{
	"connection reset",
	"connection refused",
	"connection closed",
	"broken pipe",
	"unexpected eof",
	"eof",
	"no such host",
	"tls:",
	"i/o timeout",
	"timed out",
	"http2:",
	"stream error",
}

// isNetworkish heuristically recognizes transport-level failures from wrapped
// provider errors that do not carry an HTTP status. Conservative: unknown
// messages are not treated as network failures, so they surface to the user
// instead of silently burning a fallback candidate.
func isNetworkish(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, marker := range networkMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
