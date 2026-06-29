package agent

import (
	"strings"
	"time"

	"tianxuan/internal/event"
)

// streamBatcher merges consecutive small text/reasoning chunks into fewer,
// larger Emit calls, reducing sink pressure by ~50% during DeepSeek's
// character-by-character SSE streaming.
//
// Text and reasoning accumulate independently (different event types).
// Non-text/reasoning events (tool calls, usage, errors) trigger an immediate
// flush so the dispatch pipeline is never delayed.
//
// maxBytes (8) and maxDelay (4ms) are tuned for visual smoothness: each flush
// emits 2-3 tokens at typical speeds, well below human flicker-fusion (~16ms).
//
// 缓存安全: 纯输出端优化，不改变进入 API 消息数组的任何内容。
type streamBatcher struct {
	sink event.Sink

	// text accumulator
	textBuf    strings.Builder
	textLast   time.Time

	// reasoning accumulator
	reasoningBuf  strings.Builder
	reasoningLast time.Time

	// flush triggers
	maxBytes  int           // flush when accumulated bytes exceed this (8)
	maxDelay  time.Duration // flush when oldest chunk is older than this (4ms)
}

// newStreamBatcher creates a batcher that flushes every maxDelay or when
// accumulated text exceeds maxBytes.
func newStreamBatcher(sink event.Sink) *streamBatcher {
	return &streamBatcher{
		sink:     sink,
		maxBytes: 8,
		maxDelay: 4 * time.Millisecond,
	}
}

// addText queues a text chunk. If the buffer exceeds maxBytes, the oldest
// queued chunk is older than maxDelay, or the chunk contains a newline
// (natural visual break), emits a batched Text event immediately.
func (b *streamBatcher) addText(s string) {
	now := time.Now()
	if b.textBuf.Len() == 0 {
		b.textLast = now
	}
	b.textBuf.WriteString(s)
	if b.textBuf.Len() >= b.maxBytes || now.Sub(b.textLast) >= b.maxDelay || strings.Contains(s, "\n") {
		b.flushText()
	}
}

// addReasoning queues a reasoning chunk.
func (b *streamBatcher) addReasoning(s string) {
	now := time.Now()
	if b.reasoningBuf.Len() == 0 {
		b.reasoningLast = now
	}
	b.reasoningBuf.WriteString(s)
	if b.reasoningBuf.Len() >= b.maxBytes || now.Sub(b.reasoningLast) >= b.maxDelay {
		b.flushReasoning()
	}
}

// flushAll emits any remaining buffered text and reasoning. Call once at
// the end of the stream.
func (b *streamBatcher) flushAll() {
	b.flushReasoning()
	b.flushText()
}

// flushNow flushes everything and also forces the next add* to start a fresh
// timer. Call before emitting a non-text/reasoning event (tool call, usage,
// error) so the dispatch pipeline is never delayed.
func (b *streamBatcher) flushNow() {
	b.flushReasoning()
	b.flushText()
	b.textLast = time.Time{}
	b.reasoningLast = time.Time{}
}

func (b *streamBatcher) flushText() {
	if b.textBuf.Len() == 0 {
		return
	}
	b.sink.Emit(event.Event{Kind: event.Text, Text: b.textBuf.String()})
	b.textBuf.Reset()
}

func (b *streamBatcher) flushReasoning() {
	if b.reasoningBuf.Len() == 0 {
		return
	}
	b.sink.Emit(event.Event{Kind: event.Reasoning, Text: b.reasoningBuf.String()})
	b.reasoningBuf.Reset()
}
