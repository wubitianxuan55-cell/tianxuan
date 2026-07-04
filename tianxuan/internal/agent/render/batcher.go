package render

import (
	"strings"
	"time"

	"tianxuan/internal/event"
)

// Batcher merges consecutive small text/reasoning chunks into fewer, larger
// Emit calls, reducing sink pressure during character-by-character SSE streaming.
type Batcher struct {
	sink event.Sink

	textBuf    strings.Builder
	textLast   time.Time

	reasoningBuf  strings.Builder
	reasoningLast time.Time

	maxBytes int
	maxDelay time.Duration
}

// NewBatcher creates a batcher that flushes every maxDelay or when
// accumulated text exceeds maxBytes.
func NewBatcher(sink event.Sink) *Batcher {
	return &Batcher{
		sink:     sink,
		maxBytes: 32,
		maxDelay: 4 * time.Millisecond,
	}
}

// AddText queues a text chunk.
func (b *Batcher) AddText(s string) {
	now := time.Now()
	if b.textBuf.Len() == 0 {
		b.textLast = now
	}
	b.textBuf.WriteString(s)
	if b.textBuf.Len() >= b.maxBytes || now.Sub(b.textLast) >= b.maxDelay || byteContainsNewline(s) {
		b.flushText()
	}
}

// AddReasoning queues a reasoning chunk.
func (b *Batcher) AddReasoning(s string) {
	now := time.Now()
	if b.reasoningBuf.Len() == 0 {
		b.reasoningLast = now
	}
	b.reasoningBuf.WriteString(s)
	if b.reasoningBuf.Len() >= b.maxBytes || now.Sub(b.reasoningLast) >= b.maxDelay {
		b.flushReasoning()
	}
}

// FlushAll emits any remaining buffered text and reasoning.
func (b *Batcher) FlushAll() {
	b.flushReasoning()
	b.flushText()
}

// FlushNow flushes everything and resets timers.
func (b *Batcher) FlushNow() {
	b.flushReasoning()
	b.flushText()
	b.textLast = time.Time{}
	b.reasoningLast = time.Time{}
}

func (b *Batcher) flushText() {
	if b.textBuf.Len() == 0 {
		return
	}
	b.sink.Emit(event.Event{Kind: event.Text, Text: b.textBuf.String()})
	b.textBuf.Reset()
}

func (b *Batcher) flushReasoning() {
	if b.reasoningBuf.Len() == 0 {
		return
	}
	b.sink.Emit(event.Event{Kind: event.Reasoning, Text: b.reasoningBuf.String()})
	b.reasoningBuf.Reset()
}

func byteContainsNewline(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return true
		}
	}
	return false
}
