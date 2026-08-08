package cli

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"

	"tianxuan/internal/event"
)

// newTestChatTUI builds a chatTUI with just the pieces the streaming/commit and
// completion paths need, for unit tests that don't run the bubbletea loop.
func newTestChatTUI() chatTUI {
	commit := []string{}
	ti := textarea.New()
	ti.SetWidth(80)
	return chatTUI{
		input:            ti,
		nextPasteID:      1,
		reasoningLineIdx: -1,
		answerIdx:        -1,
		reasoning:        &strings.Builder{},
		pending:          &strings.Builder{},
		pendingCommit:    &commit,
		renderer:         newMarkdownRenderer(80),
	}
}

// TestIngestSeparatesReasoningFromAnswer proves the thinking marker appears the
// moment reasoning starts, collapses in place to a "thought for Ns" summary when
// the answer begins, and the answer commits as its own distinct entry.
func TestIngestSeparatesReasoningFromAnswer(t *testing.T) {
	m := newTestChatTUI()

	m.ingestEvent(event.Event{Kind: event.Reasoning, Text: "…reasoning…"}) // thinking starts → live marker
	if len(m.transcript) != 1 || !strings.Contains(m.transcript[0], "thinking") {
		t.Fatalf("thinking marker should appear at once, transcript=%v", m.transcript)
	}
	if strings.Contains(m.transcript[0], "…reasoning…") {
		t.Fatalf("raw reasoning text should stay collapsed by default, transcript=%v", m.transcript)
	}

	m.ingestEvent(event.Event{Kind: event.Text, Text: "Hello answer"}) // answer begins → marker collapses
	if len(m.transcript) != 1 || !strings.Contains(m.transcript[0], "thought for") {
		t.Fatalf("marker should collapse to a duration summary in place, transcript=%v", m.transcript)
	}
	if m.pending.String() != "Hello answer" {
		t.Errorf("answer should be live in pending, got %q", m.pending.String())
	}
	if m.reasoning.Len() != 0 {
		t.Errorf("reasoning buffer should be cleared after commit")
	}

	m.commitPending() // turn end
	if len(m.transcript) != 2 || !strings.Contains(m.transcript[1], "Hello") {
		t.Fatalf("answer should commit as a separate entry, transcript=%v", m.transcript)
	}
}

// TestVerboseReasoningInsertsTextUnderSummary proves /verbose mode keeps the full
// thinking text, placed beneath the collapsed duration summary.
func TestVerboseReasoningInsertsTextUnderSummary(t *testing.T) {
	m := newTestChatTUI()
	m.showReasoning = true

	m.ingestEvent(event.Event{Kind: event.Reasoning, Text: "step one "})
	m.ingestEvent(event.Event{Kind: event.Reasoning, Text: "step two"})
	m.ingestEvent(event.Event{Kind: event.Text, Text: "Answer"}) // closes the block

	if len(m.transcript) != 2 {
		t.Fatalf("verbose block should be summary + text, transcript=%v", m.transcript)
	}
	if !strings.Contains(m.transcript[0], "thought for") {
		t.Errorf("first line should be the duration summary, got %q", m.transcript[0])
	}
	if !strings.Contains(m.transcript[1], "step one") || !strings.Contains(m.transcript[1], "step two") {
		t.Errorf("verbose text should appear under the summary, got %q", m.transcript[1])
	}
}

// TestIngestEventFlushesAnswer confirms an event line (e.g. a tool dispatch)
// finalizes the answer streamed before it, preserving order in scrollback.
func TestIngestEventFlushesAnswer(t *testing.T) {
	m := newTestChatTUI()
	m.ingestEvent(event.Event{Kind: event.Text, Text: "partial answer "})
	m.ingestEvent(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{Name: "read_file", Args: `{"path":"x"}`}})
	if n := len(*m.pendingCommit); n != 2 {
		t.Fatalf("answer then event line should be two commits, got %d: %v", n, *m.pendingCommit)
	}
	if !strings.Contains((*m.pendingCommit)[0], "partial answer") {
		t.Errorf("first commit should be the buffered answer, got %q", (*m.pendingCommit)[0])
	}
	if !strings.Contains((*m.pendingCommit)[1], "-> read_file") {
		t.Errorf("second commit should be the event line, got %q", (*m.pendingCommit)[1])
	}
	if m.pending.Len() != 0 {
		t.Errorf("answer buffer should be drained after the event line")
	}
}

// TestStreamAnswerFlushesCompletedParagraphs proves a multi-paragraph answer
// appears chunk by chunk: a closed paragraph renders to scrollback while the
// still-streaming one stays buffered, and turn end flushes the remainder.
func TestStreamAnswerFlushesCompletedParagraphs(t *testing.T) {
	m := newTestChatTUI()

	m.ingestEvent(event.Event{Kind: event.Text, Text: "First paragraph.\n\nSecond para "})
	if m.answerIdx < 0 {
		t.Fatalf("a completed paragraph should open a streamed answer block")
	}
	joined := strings.Join(m.transcript, "\n")
	if !strings.Contains(joined, "First paragraph.") {
		t.Errorf("completed paragraph should be on screen, transcript=%v", m.transcript)
	}
	if strings.Contains(joined, "Second para") {
		t.Errorf("the still-streaming paragraph must stay buffered, transcript=%v", m.transcript)
	}

	m.ingestEvent(event.Event{Kind: event.Text, Text: "is done now."})
	m.ingestEvent(event.Event{Kind: event.Message})
	final := strings.Join(m.transcript, "\n")
	if !strings.Contains(final, "First paragraph.") || !strings.Contains(final, "Second para is done now.") {
		t.Errorf("turn end should flush the whole answer, transcript=%v", m.transcript)
	}
	if m.pending.Len() != 0 || m.answerIdx != -1 {
		t.Errorf("answer state should reset after commit, pending=%d idx=%d", m.pending.Len(), m.answerIdx)
	}
}

// TestFlushableMarkdownPrefixKeepsOpenFence proves a blank line inside an unclosed
// fenced code block is not a flush boundary — the half-written block stays buffered
// so it never renders mangled, while prose before the fence does flush.
func TestFlushableMarkdownPrefixKeepsOpenFence(t *testing.T) {
	open := "intro line\n\n```go\nfunc f() {\n\n\t// still typing"
	if got := flushableMarkdownPrefix(open); got != "intro line" {
		t.Errorf("open fence: flushable prefix = %q, want %q", got, "intro line")
	}

	closed := "```go\ncode\n\nmore\n```\n\ntrailing"
	if got := flushableMarkdownPrefix(closed); got != "```go\ncode\n\nmore\n```" {
		t.Errorf("closed fence: flushable prefix = %q", got)
	}

	if got := flushableMarkdownPrefix("no boundary yet"); got != "" {
		t.Errorf("no blank line should flush nothing, got %q", got)
	}
}

// TestFlushableMarkdownPrefixFlushesLinesWithoutBlankLine proves a reply that
// has not yet closed a paragraph (no blank line) still renders incrementally:
// completed lines flush while the trailing half-written line stays buffered,
// and a lone line flushes once it is wide enough to feel "live". A half-written
// fence keeps its whole block buffered either way.
func TestFlushableMarkdownPrefixFlushesLinesWithoutBlankLine(t *testing.T) {
	multiShortTail := "- one\n- two\n- thr"
	if got := flushableMarkdownPrefix(multiShortTail); got != "- one\n- two" {
		t.Errorf("multi-line, short tail: flushable prefix = %q, want %q", got, "- one\n- two")
	}

	longTail := "- one\n- two\n" + strings.Repeat("x", 100)
	if got := flushableMarkdownPrefix(longTail); got != longTail {
		t.Errorf("multi-line, wide tail: flushable prefix = %q, want whole buffer", got)
	}

	loneLine := strings.Repeat("x", 100)
	if got := flushableMarkdownPrefix(loneLine); got != loneLine {
		t.Errorf("lone wide line: flushable prefix = %q, want the line", got)
	}

	fenced := "```go\n" + strings.Repeat("x", 100) + "\nstill open"
	if got := flushableMarkdownPrefix(fenced); got != "" {
		t.Errorf("open fence: flushable prefix = %q, want nothing", got)
	}
}

// TestStreamAnswerThrottlesIncrementalRenders proves a flushed line that grows
// token by token is not re-rendered on every event: small increments stay
// buffered until the growth floor is crossed, so long replies don't re-parse
// the whole answer on each token.
func TestStreamAnswerThrottlesIncrementalRenders(t *testing.T) {
	m := newTestChatTUI()

	m.ingestEvent(event.Event{Kind: event.Text, Text: strings.Repeat("x", 100)})
	if m.answerIdx < 0 {
		t.Fatalf("lone wide line should open a streamed answer block")
	}
	flushed := m.answerFlushed

	m.ingestEvent(event.Event{Kind: event.Text, Text: strings.Repeat("y", 10)})
	if m.answerFlushed != flushed {
		t.Errorf("small increment should stay buffered, flushed %d -> %d", flushed, m.answerFlushed)
	}

	m.ingestEvent(event.Event{Kind: event.Text, Text: strings.Repeat("z", 40)})
	if m.answerFlushed != flushed+50 {
		t.Errorf("growth past the floor should re-render, flushed %d -> %d", flushed, m.answerFlushed)
	}
}
