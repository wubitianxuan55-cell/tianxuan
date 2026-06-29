package agent

import (
	"strings"
	"testing"
	"time"

	"tianxuan/internal/event"
)

// batchSink captures emitted evts for inspection.
type batchSink struct {
	evts []event.Event
}

func (s *batchSink) Emit(e event.Event) {
	s.evts = append(s.evts, e)
}

func TestStreamBatcherFlushOnSize(t *testing.T) {
	sink := &batchSink{}
	b := newStreamBatcher(sink)
	b.maxBytes = 64              // override default (8) for this test
	b.maxDelay = time.Hour       // disable time-based flush

	// Add small chunks — should NOT emit until 64 bytes.
	b.addText("hello ")
	if len(sink.evts) != 0 {
		t.Fatal("should not flush below maxBytes")
	}
	b.addText("world")
	if len(sink.evts) != 0 {
		t.Fatal("still below maxBytes")
	}

	// This push exceeds 64 bytes.
	b.addText(strings.Repeat("x", 60))
	if len(sink.evts) != 1 {
		t.Fatalf("expected 1 flush, got %d evts", len(sink.evts))
	}
	if sink.evts[0].Kind != event.Text {
		t.Fatalf("expected Text event, got %v", sink.evts[0].Kind)
	}
}

func TestStreamBatcherFlushOnTime(t *testing.T) {
	sink := &batchSink{}
	b := newStreamBatcher(sink)
	b.maxBytes = 9999            // disable size-based flush
	b.maxDelay = 0               // always flush immediately
	b.addText("tiny")
	if len(sink.evts) != 1 {
		t.Fatalf("expected immediate flush on zero delay, got %d", len(sink.evts))
	}
}

func TestStreamBatcherSeparateTextAndReasoning(t *testing.T) {
	sink := &batchSink{}
	b := newStreamBatcher(sink)
	b.maxDelay = time.Hour

	b.addText("hello")
	b.addReasoning("thinking...")
	b.flushAll()

	if len(sink.evts) != 2 {
		t.Fatalf("expected 2 evts, got %d", len(sink.evts))
	}
	// Reasoning should come first (flushed first in flushAll).
	if sink.evts[0].Kind != event.Reasoning || sink.evts[1].Kind != event.Text {
		t.Fatalf("expected [Reasoning, Text], got [%v, %v]", sink.evts[0].Kind, sink.evts[1].Kind)
	}
}

func TestStreamBatcherFlushNow(t *testing.T) {
	sink := &batchSink{}
	b := newStreamBatcher(sink)
	b.maxDelay = time.Hour

	b.addText("pending text")
	b.addReasoning("pending reasoning")
	b.flushNow()

	if len(sink.evts) != 2 {
		t.Fatalf("expected 2 evts, got %d", len(sink.evts))
	}
	// After flushNow, buffers are empty.
	b.flushAll()
	if len(sink.evts) != 2 {
		t.Fatal("flushAll after flushNow should be no-op")
	}
}

func TestStreamBatcherEmptyFlushNoop(t *testing.T) {
	sink := &batchSink{}
	b := newStreamBatcher(sink)
	b.flushAll()
	b.flushNow()
	b.flushText()
	b.flushReasoning()
	if len(sink.evts) != 0 {
		t.Fatalf("empty batcher should not emit, got %d", len(sink.evts))
	}
}

func TestStreamBatcherFlushAllPreservesOrder(t *testing.T) {
	sink := &batchSink{}
	b := newStreamBatcher(sink)
	b.maxDelay = time.Hour

	// Interleave text and reasoning.
	b.addText("a")
	b.addReasoning("1")
	b.addText("b")
	b.addReasoning("2")
	b.flushAll()

	if len(sink.evts) != 2 {
		t.Fatalf("expected 2 batched evts, got %d", len(sink.evts))
	}
	if sink.evts[0].Text != "12" {
		t.Fatalf("reasoning batch wrong: %q", sink.evts[0].Text)
	}
	if sink.evts[1].Text != "ab" {
		t.Fatalf("text batch wrong: %q", sink.evts[1].Text)
	}
}
