// Package agent — V3.3: distributed trace ID for end-to-end observability.
// Every turn gets a unique TraceID that propagates through Planner,
// ToolDispatcher, LLM calls, and Compaction, enabling per-turn log aggregation.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type traceIDKey struct{}

// NewTraceID generates a 16-byte random trace ID (hex-encoded, 32 chars).
func NewTraceID() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// WithTraceID stores a trace ID in the context.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}

// TraceID extracts the trace ID from the context.
// Returns "" when no trace ID is set.
func TraceID(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey{}).(string); ok {
		return id
	}
	return ""
}
