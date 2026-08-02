package failover

import (
	"context"
	"fmt"

	"tianxuan/internal/provider"
)

// Options configures a Chain.
type Options struct {
	// MaxChainRetries is how many extra whole-chain rounds run when every
	// candidate failed with a pure overload error (OpenClaw retries the chain
	// up to 10 times; the default 2 keeps the ceiling modest for CLI turns).
	MaxChainRetries int
	// Backoff is the exponential backoff between whole-chain rounds. Zero uses
	// provider.RateLimitBackoff (tuned for 429 responses).
	Backoff provider.BackoffStrategy
	// OnSwitch fires once when a turn moves onto a fallback candidate, with
	// the primary label, the winning candidate label, and the failure that
	// caused the switch. Used to surface a model-fallback notice.
	OnSwitch func(from, to string, err error)
}

type candidate struct {
	prov  provider.Provider
	label string
}

// Chain is a turn-local fallback chain over model candidates. It implements
// provider.Provider, so callers keep using one provider handle and the chain
// decides internally which candidate answers. Normal turns stay on the
// primary (cache-stable); only failover-worthy errors advance the chain.
type Chain struct {
	candidates []candidate
	maxRetries int
	backoff    provider.BackoffStrategy
	onSwitch   func(from, to string, err error)
}

// New builds a chain with primary first, then each fallback in order. Nil
// providers are skipped. The chain's Name is the primary's name so
// telemetry/UI keep reporting the selected model.
func New(primary provider.Provider, fallbacks []provider.Provider, opts Options) *Chain {
	cands := make([]candidate, 0, 1+len(fallbacks))
	if primary != nil {
		cands = append(cands, candidate{prov: primary, label: primary.Name()})
	}
	for _, f := range fallbacks {
		if f == nil {
			continue
		}
		cands = append(cands, candidate{prov: f, label: f.Name()})
	}
	maxRetries := opts.MaxChainRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	backoff := opts.Backoff
	if backoff.Base <= 0 && backoff.Max <= 0 {
		backoff = provider.RateLimitBackoff()
	}
	return &Chain{
		candidates: cands,
		maxRetries: maxRetries,
		backoff:    backoff,
		onSwitch:   opts.OnSwitch,
	}
}

// Name reports the chain's primary model name.
func (c *Chain) Name() string {
	if len(c.candidates) == 0 {
		return ""
	}
	return c.candidates[0].label
}

// Stream tries candidates in order. On a failover-worthy failure it advances
// to the next candidate; when every candidate fails with a pure overload, the
// whole chain is retried with backoff up to MaxChainRetries additional rounds.
// Exhaustion returns a FallbackSummaryError carrying per-attempt detail.
func (c *Chain) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	if len(c.candidates) == 0 {
		return nil, fmt.Errorf("failover: no model candidates configured")
	}
	var attempts []Attempt
	primaryLabel := c.candidates[0].label
	for round := 0; round <= c.maxRetries; round++ {
		roundStart := len(attempts)
		for i, cand := range c.candidates {
			ch, err := cand.prov.Stream(ctx, req)
			if err == nil {
				if i > 0 {
					var cause error
					if len(attempts) > 0 {
						cause = attempts[len(attempts)-1].Err
					}
					c.notifySwitch(primaryLabel, cand.label, cause)
				}
				return ch, nil
			}
			attempts = append(attempts, Attempt{Label: cand.label, Err: err})
			if !ShouldFailover(err) {
				return nil, err
			}
		}
		// Whole round failed. Retry only when every attempt this round was a
		// pure overload; other failures would repeat identically.
		if round >= c.maxRetries || !allOverload(attempts[roundStart:]) {
			break
		}
		if err := c.backoff.Sleep(ctx, round); err != nil {
			return nil, err
		}
	}
	return nil, &FallbackSummaryError{Attempts: attempts}
}

func (c *Chain) notifySwitch(from, to string, err error) {
	if c.onSwitch != nil {
		c.onSwitch(from, to, err)
	}
}

func allOverload(attempts []Attempt) bool {
	if len(attempts) == 0 {
		return false
	}
	for _, a := range attempts {
		if !IsOverload(a.Err) {
			return false
		}
	}
	return true
}

var _ provider.Provider = (*Chain)(nil)
