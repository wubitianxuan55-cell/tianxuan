package failover

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"tianxuan/internal/provider"
)

// ─── V10.150: Model Failover（蒸馏自 OpenClaw model-failover）───
//
// primary 模型限流/过载/断网时按序切备用模型，turn-local 语义：
// 回退只用于当前回合，下回合回到 primary。纯过载时整链退避重试。

// statusErr carries an HTTP status code for classification tests.
type statusErr struct {
	code int
	msg  string
}

func (e statusErr) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return fmt.Sprintf("status %d", e.code)
}

func (e statusErr) HTTPStatus() int { return e.code }

// fakeProv records call counts and returns a fixed error or a valid stream.
type fakeProv struct {
	name  string
	err   error
	calls atomic.Int64
}

func (f *fakeProv) Name() string { return f.name }

func (f *fakeProv) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan provider.Chunk, 1)
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func noBackoff() provider.BackoffStrategy {
	// Base=1ns: Chain treats zero Base/Max as "unset" and installs the
	// default RateLimitBackoff; 1ns keeps whole-chain retries immediate.
	return provider.BackoffStrategy{Base: 1}
}

func TestChain_PrimarySucceeds(t *testing.T) {
	primary := &fakeProv{name: "p"}
	fallback := &fakeProv{name: "f"}
	c := New(primary, []provider.Provider{fallback}, Options{Backoff: noBackoff()})

	if c.Name() != "p" {
		t.Errorf("Name() = %q, want %q", c.Name(), "p")
	}
	ch, err := c.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range ch {
	}
	if primary.calls.Load() != 1 {
		t.Errorf("primary calls = %d, want 1", primary.calls.Load())
	}
	if fallback.calls.Load() != 0 {
		t.Errorf("fallback must not be called on success, got %d", fallback.calls.Load())
	}
}

func TestChain_FailoverOnRetryable(t *testing.T) {
	primary := &fakeProv{name: "p", err: statusErr{code: 429}}
	fallback := &fakeProv{name: "f"}
	var switches []string
	c := New(primary, []provider.Provider{fallback}, Options{
		Backoff: noBackoff(),
		OnSwitch: func(from, to string, _ error) {
			switches = append(switches, from+"->"+to)
		},
	})
	ch, err := c.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range ch {
	}
	if fallback.calls.Load() != 1 {
		t.Errorf("fallback calls = %d, want 1", fallback.calls.Load())
	}
	if len(switches) != 1 || switches[0] != "p->f" {
		t.Errorf("onSwitch = %v, want [p->f]", switches)
	}
}

func TestChain_NoFailoverOnParamError(t *testing.T) {
	primary := &fakeProv{name: "p", err: statusErr{code: 400}}
	fallback := &fakeProv{name: "f"}
	c := New(primary, []provider.Provider{fallback}, Options{Backoff: noBackoff()})
	_, err := c.Stream(context.Background(), provider.Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	if fallback.calls.Load() != 0 {
		t.Errorf("fallback must not run for non-failover-worthy errors, got %d calls", fallback.calls.Load())
	}
}

func TestChain_AllFailReturnsSummary(t *testing.T) {
	primary := &fakeProv{name: "p", err: statusErr{code: 429}}
	fallback := &fakeProv{name: "f", err: statusErr{code: 503}}
	c := New(primary, []provider.Provider{fallback}, Options{Backoff: noBackoff(), MaxChainRetries: 0})
	_, err := c.Stream(context.Background(), provider.Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	var summary *FallbackSummaryError
	if !errors.As(err, &summary) {
		t.Fatalf("want *FallbackSummaryError, got %T: %v", err, err)
	}
	if len(summary.Attempts) != 2 {
		t.Errorf("attempts = %d, want 2", len(summary.Attempts))
	}
	if summary.Attempts[0].Label != "p" || summary.Attempts[1].Label != "f" {
		t.Errorf("attempt order wrong: %+v", summary.Attempts)
	}
}

func TestChain_OverloadRetriesWholeChain(t *testing.T) {
	// 两轮全败（503），第三轮 primary 成功：MaxChainRetries=2 时整链重试生效。
	primary := &fakeProv{name: "p", err: statusErr{code: 503}}
	fallback := &fakeProv{name: "f", err: statusErr{code: 503}}
	c := New(primary, []provider.Provider{fallback}, Options{
		Backoff:         noBackoff(),
		MaxChainRetries: 2,
	})
	_, err := c.Stream(context.Background(), provider.Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	// 全部候选都是固定错误，重试 3 轮后仍失败 → FallbackSummaryError。
	var summary *FallbackSummaryError
	if !errors.As(err, &summary) {
		t.Fatalf("want summary error, got %T: %v", err, err)
	}
	if primary.calls.Load() != 3 || fallback.calls.Load() != 3 {
		t.Errorf("chain retry count wrong: primary=%d fallback=%d, want 3 each",
			primary.calls.Load(), fallback.calls.Load())
	}
}

func TestChain_OverloadRecoversOnRetry(t *testing.T) {
	// 第一轮全败（503），第二轮 primary 成功 → 链重试后恢复。
	primary := &fakeProv{name: "p"}
	fallback := &fakeProv{name: "f", err: statusErr{code: 503}}
	// 用包装：前 2 次调用失败，之后成功。
	flaky := &flakyProv{prov: primary, failFirst: 2, err: statusErr{code: 503}}
	c := New(flaky, []provider.Provider{fallback}, Options{
		Backoff:         noBackoff(),
		MaxChainRetries: 3,
	})
	ch, err := c.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("Stream should recover on chain retry: %v", err)
	}
	for range ch {
	}
	if flaky.calls.Load() != 3 {
		t.Errorf("primary calls = %d, want 3 (1 initial + 2 retries)", flaky.calls.Load())
	}
}

func TestChain_CancelNotFailover(t *testing.T) {
	primary := &fakeProv{name: "p", err: context.Canceled}
	fallback := &fakeProv{name: "f"}
	c := New(primary, []provider.Provider{fallback}, Options{Backoff: noBackoff(), MaxChainRetries: 2})
	_, err := c.Stream(context.Background(), provider.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel must propagate, got %v", err)
	}
	if fallback.calls.Load() != 0 {
		t.Errorf("fallback must not run on cancellation, got %d calls", fallback.calls.Load())
	}
}

func TestChain_AuthErrorNotFailover(t *testing.T) {
	primary := &fakeProv{name: "p", err: &provider.AuthError{Provider: "p", Status: 401}}
	fallback := &fakeProv{name: "f"}
	c := New(primary, []provider.Provider{fallback}, Options{Backoff: noBackoff()})
	_, err := c.Stream(context.Background(), provider.Request{})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if fallback.calls.Load() != 0 {
		t.Errorf("fallback must not run on auth failure (same key), got %d calls", fallback.calls.Load())
	}
}

// flakyProv fails the first N Stream calls, then delegates to the inner provider.
type flakyProv struct {
	prov      *fakeProv
	failFirst int64
	err       error
	calls     atomic.Int64
}

func (f *flakyProv) Name() string { return f.prov.name }

func (f *flakyProv) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	n := f.calls.Add(1)
	if n <= f.failFirst {
		return nil, f.err
	}
	return f.prov.Stream(ctx, req)
}

func TestShouldFailover(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"rate limit 429", statusErr{code: 429}, true},
		{"server 500", statusErr{code: 500}, true},
		{"bad gateway 502", statusErr{code: 502}, true},
		{"service unavailable 503", statusErr{code: 503}, true},
		{"param error 400", statusErr{code: 400}, false},
		{"not found 404", statusErr{code: 404}, false},
		{"conflict 409", statusErr{code: 409}, false},
		{"auth 401", &provider.AuthError{Status: 401}, false},
		{"auth 403", &provider.AuthError{Status: 403}, false},
		{"cancel", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
		{"connection reset", errors.New("openai: request failed: connection reset by peer"), true},
		{"connection refused", errors.New("connection refused"), true},
		{"EOF", errors.New("unexpected EOF"), true},
		{"DNS", errors.New("no such host"), true},
		{"TLS", errors.New("tls: handshake failure"), true},
		{"generic error", errors.New("boom"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldFailover(tc.err); got != tc.want {
				t.Errorf("ShouldFailover(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestChain_SwitchNoticeIncludesReason(t *testing.T) {
	primary := &fakeProv{name: "p", err: statusErr{code: 429, msg: "rate limited"}}
	fallback := &fakeProv{name: "f"}
	var reason string
	c := New(primary, []provider.Provider{fallback}, Options{
		Backoff: noBackoff(),
		OnSwitch: func(_, _ string, err error) {
			reason = err.Error()
		},
	})
	ch, err := c.Stream(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range ch {
	}
	if !strings.Contains(reason, "rate limited") {
		t.Errorf("switch reason should mention the failure, got %q", reason)
	}
}

func TestChain_NoFallbacks(t *testing.T) {
	primary := &fakeProv{name: "p", err: statusErr{code: 503}}
	c := New(primary, nil, Options{Backoff: noBackoff(), MaxChainRetries: 1})
	_, err := c.Stream(context.Background(), provider.Request{})
	var summary *FallbackSummaryError
	if !errors.As(err, &summary) {
		t.Fatalf("want summary error with single candidate, got %T: %v", err, err)
	}
	if len(summary.Attempts) != 2 {
		t.Errorf("attempts = %d, want 2 (initial + 1 retry)", len(summary.Attempts))
	}
}
