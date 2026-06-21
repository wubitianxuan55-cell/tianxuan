package provider

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestBackoffStrategy_Duration(t *testing.T) {
	tests := []struct {
		name    string
		b       BackoffStrategy
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:    "default attempt 0 (first retry)",
			b:       DefaultBackoff(),
			attempt: 0,
			wantMin: 0,
			wantMax: 500 * time.Millisecond,
		},
		{
			name:    "default attempt 1",
			b:       DefaultBackoff(),
			attempt: 1,
			wantMin: 0,
			wantMax: 1 * time.Second,
		},
		{
			name:    "default capped at max",
			b:       DefaultBackoff(),
			attempt: 10,
			wantMin: 0,
			wantMax: 8 * time.Second,
		},
		{
			name:    "rate limit attempt 0",
			b:       RateLimitBackoff(),
			attempt: 0,
			wantMin: 0,
			wantMax: 5 * time.Second,
		},
		{
			name:    "rate limit capped at max",
			b:       RateLimitBackoff(),
			attempt: 10,
			wantMin: 0,
			wantMax: 60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.b.Duration(tt.attempt)
			if d < tt.wantMin || d > tt.wantMax {
				t.Errorf("Duration(%d) = %v, want [%v, %v]", tt.attempt, d, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestBackoffStrategy_Deterministic(t *testing.T) {
	// While Duration() contains randomness (jitter), it should always stay within bounds.
	b := DefaultBackoff()
	for i := 0; i < 100; i++ {
		d := b.Duration(i)
		if d < 0 || d > b.Max {
			t.Errorf("attempt %d: Duration = %v exceeds bounds [0, %v]", i, d, b.Max)
		}
	}
}

func TestBackoffStrategy_Sleep_CtxCancel(t *testing.T) {
	b := BackoffStrategy{
		Base:       10 * time.Second,
		Max:        10 * time.Second,
		Multiplier: 1.0,
		Jitter:     false,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := b.Sleep(ctx, 0)
	if err == nil {
		t.Error("Sleep with cancelled ctx should return an error")
	}
}

func TestBackoffStrategy_Sleep_Zero(t *testing.T) {
	b := BackoffStrategy{
		Base:       0,
		Max:        0,
		Multiplier: 0,
		Jitter:     false,
	}
	err := b.Sleep(context.Background(), 0)
	if err != nil {
		t.Errorf("Sleep with zero duration should succeed: %v", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		header string
		cap    time.Duration
		want   time.Duration
	}{
		{"seconds", "30", 120 * time.Second, 30 * time.Second},
		{"seconds at cap", "200", 120 * time.Second, 120 * time.Second},
		{"empty", "", 120 * time.Second, 0},
		{"zero", "0", 120 * time.Second, 0},
		{"negative", "-5", 120 * time.Second, 0},
		{"garbage", "not-a-number", 120 * time.Second, 0},
		{"http date", "Mon, 02 Jan 2006 15:04:05 GMT", 120 * time.Second, 0}, // past date → 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.header != "" {
				resp.Header.Set("Retry-After", tt.header)
			}
			got := ParseRetryAfter(resp, tt.cap)
			if got != tt.want {
				t.Errorf("ParseRetryAfter(%q, %v) = %v, want %v", tt.header, tt.cap, got, tt.want)
			}
		})
	}
}

func TestParseRetryAfter_NilResponse(t *testing.T) {
	if got := ParseRetryAfter(nil, 120*time.Second); got != 0 {
		t.Errorf("ParseRetryAfter(nil) = %v, want 0", got)
	}
}

func TestIsRetryableStatus(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{408, true},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}

	for _, tt := range tests {
		if got := IsRetryableStatus(tt.code); got != tt.want {
			t.Errorf("IsRetryableStatus(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestIsTransientNetErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"canceled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTransientNetErr(tt.err); got != tt.want {
				t.Errorf("IsTransientNetErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
