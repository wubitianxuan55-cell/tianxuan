package render

import (
	"strings"
	"testing"
	"time"

	"tianxuan/internal/event"
)

// batchSink captures emitted events for inspection.
type batchSink struct {
	evts []event.Event
}

func (s *batchSink) Emit(e event.Event) {
	s.evts = append(s.evts, e)
}

func TestBatcherFlushOnSize(t *testing.T) {
	sink := &batchSink{}
	b := NewBatcher(sink)
	b.maxBytes = 64
	b.maxDelay = time.Hour

	b.AddText("hello ")
	if len(sink.evts) != 0 {
		t.Fatal("should not flush below maxBytes")
	}
	b.AddText("world")
	if len(sink.evts) != 0 {
		t.Fatal("still below maxBytes")
	}

	b.AddText(strings.Repeat("x", 60))
	if len(sink.evts) != 1 {
		t.Fatalf("expected 1 flush, got %d evts", len(sink.evts))
	}
	if sink.evts[0].Kind != event.Text {
		t.Fatalf("expected Text event, got %v", sink.evts[0].Kind)
	}
}

func TestBatcherFlushOnTime(t *testing.T) {
	sink := &batchSink{}
	b := NewBatcher(sink)
	b.maxBytes = 9999
	b.maxDelay = 0
	b.AddText("tiny")
	if len(sink.evts) != 1 {
		t.Fatalf("expected immediate flush on zero delay, got %d", len(sink.evts))
	}
}

func TestBatcherSeparateTextAndReasoning(t *testing.T) {
	sink := &batchSink{}
	b := NewBatcher(sink)
	b.maxDelay = time.Hour

	b.AddText("hello")
	b.AddReasoning("thinking...")
	b.FlushAll()

	if len(sink.evts) != 2 {
		t.Fatalf("expected 2 evts, got %d", len(sink.evts))
	}
	if sink.evts[0].Kind != event.Reasoning || sink.evts[1].Kind != event.Text {
		t.Fatalf("expected [Reasoning, Text], got [%v, %v]", sink.evts[0].Kind, sink.evts[1].Kind)
	}
}

func TestBatcherFlushNow(t *testing.T) {
	sink := &batchSink{}
	b := NewBatcher(sink)
	b.maxDelay = time.Hour

	b.AddText("pending text")
	b.AddReasoning("pending reasoning")
	b.FlushNow()

	if len(sink.evts) != 2 {
		t.Fatalf("expected 2 evts, got %d", len(sink.evts))
	}
	b.FlushAll()
	if len(sink.evts) != 2 {
		t.Fatal("FlushAll after FlushNow should be no-op")
	}
}

func TestBatcherEmptyFlushNoop(t *testing.T) {
	sink := &batchSink{}
	b := NewBatcher(sink)
	b.FlushAll()
	b.FlushNow()
	if len(sink.evts) != 0 {
		t.Fatalf("empty batcher should not emit, got %d", len(sink.evts))
	}
}

func TestBatcherFlushAllPreservesOrder(t *testing.T) {
	sink := &batchSink{}
	b := NewBatcher(sink)
	b.maxDelay = time.Hour

	b.AddText("a")
	b.AddReasoning("1")
	b.AddText("b")
	b.AddReasoning("2")
	b.FlushAll()

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
