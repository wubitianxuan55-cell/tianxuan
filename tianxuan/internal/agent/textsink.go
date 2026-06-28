package agent

import (
	"fmt"
	"io"
	"strings"
	"time"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
)

// Pre-computed ANSI dim SGR sequences — avoid per-chunk string allocation
// in the hot reasoning-rendering path.
var (
	dimPrefix = []byte("\x1b[2m")
	dimSuffix = []byte("\x1b[0m")
	newline   = []byte("\n")
)

// TextSink renders a turn's event stream to ANSI text on an io.Writer.
type TextSink struct {
	out       io.Writer
	renderer  Renderer
	termWidth int

	wroteReasoningHeader bool
	wroteReasoningBody   bool
	textWritten          bool
	showReasoning        bool
	wroteAnything        bool

	// R3.1: reasoning throttle — show progress indicator instead of full text
	reasoningChars     int
	reasoningLastFlush time.Time
	reasoningActive    bool // true while \r-overwritable progress line is live

	// R3.2: tool batch — merge rapid tool dispatches into a summary line
	pendingTools []string

	// R3.3: error aggregation — merge consecutive errors into one line
	pendingErrors []string
}

func NewTextSink(out io.Writer, renderer Renderer, termWidth int) *TextSink {
	return &TextSink{out: out, renderer: renderer, termWidth: termWidth}
}

func (s *TextSink) SetShowReasoning(show bool) { s.showReasoning = show }

func (s *TextSink) Emit(e event.Event) {
	switch e.Kind {
	case event.TurnStarted:
		s.wroteReasoningHeader = false
		s.wroteReasoningBody = false
		s.textWritten = false
		s.wroteAnything = false
		s.reasoningChars = 0
		s.reasoningActive = false
		s.pendingTools = nil
		s.pendingErrors = nil

	case event.Reasoning:
		if !s.wroteReasoningHeader {
			writeDim(s.out, "  ▎ thinking")
			s.wroteReasoningHeader = true
			s.reasoningActive = true
			s.reasoningLastFlush = time.Now()
		}
		if s.showReasoning && e.Text != "" {
			s.reasoningChars += len(e.Text)
			s.wroteReasoningBody = true
			// Throttle: update progress every 500ms (overwrite current line with \r)
			if now := time.Now(); now.Sub(s.reasoningLastFlush) >= 500*time.Millisecond {
				s.flushReasoningProgress()
				s.reasoningLastFlush = now
			}
		}
		s.wroteAnything = true

	case event.Text:
		s.flushBatchedTools()
		s.flushAggregatedErrors()
		s.finalizeReasoning()
		if s.wroteReasoningHeader && s.wroteReasoningBody && !s.textWritten {
			s.out.Write(newline)
		}
		s.out.Write([]byte(e.Text))
		s.textWritten = true
		s.wroteAnything = true

	case event.Message:
		s.flushBatchedTools()
		s.flushAggregatedErrors()
		s.finalizeReasoning()
		s.closeTextStream(e.Text, e.Reasoning)

	case event.ToolDispatch:
		s.finalizeReasoning()
		s.flushAggregatedErrors()
		if e.Tool.Partial {
			break
		}
		s.pendingTools = append(s.pendingTools,
			fmt.Sprintf("%s %s", e.Tool.Name, CompactArgs(e.Tool.Args)))
		// Flush immediately if 3+ tools accumulated (batch already big enough to summarise)
		if len(s.pendingTools) >= 3 {
			s.flushBatchedTools()
		}
		s.wroteAnything = true

	case event.ToolResult:
		s.flushBatchedTools()
		if e.Tool.Err != "" {
			s.pendingErrors = append(s.pendingErrors,
				fmt.Sprintf("%s(%s)", e.Tool.Name, e.Tool.Err))
		}
		s.wroteAnything = true

	case event.Usage:
		s.flushBatchedTools()
		s.flushAggregatedErrors()
		s.finalizeReasoning()
		if s.textWritten {
			s.out.Write(newline)
			s.textWritten = false
		}
		s.usageLine(e.Usage, e.Pricing)

	case event.Notice:
		s.flushBatchedTools()
		s.flushAggregatedErrors()
		s.finalizeReasoning()
		glyph := "·"
		if e.Level == event.LevelWarn {
			glyph = "!"
		}
		fmt.Fprintf(s.out, "  %s %s\n", glyph, e.Text)
		s.wroteAnything = true

	case event.Phase:
		s.flushBatchedTools()
		s.flushAggregatedErrors()
		s.finalizeReasoning()
		if s.wroteAnything {
			s.out.Write(newline)
		}
		fmt.Fprintf(s.out, "[%s]\n", e.Text)
		s.wroteAnything = true

	case event.CompactionStarted:
		s.flushBatchedTools()
		s.flushAggregatedErrors()
		s.finalizeReasoning()
		writeDim(s.out, "  ⋯ compacting conversation…")
		s.out.Write(newline)
		s.wroteAnything = true

	case event.CompactionDone:
		s.flushBatchedTools()
		s.flushAggregatedErrors()
		s.finalizeReasoning()
		c := e.Compaction
		if c.Summary == "" {
			break
		}
		s2 := fmt.Sprintf("  ⋯ compacted %d messages (%s)", c.Messages, c.Trigger)
		writeDim(s.out, s2)
		s.out.Write(newline)
		for _, ln := range strings.Split(strings.TrimRight(c.Summary, "\n"), "\n") {
			writeDim(s.out, "    "+ln)
			s.out.Write(newline)
		}
		s.wroteAnything = true
	}
}

// finalizeReasoning ends the \r-overwritable reasoning progress line with a
// newline and the final character count.
func (s *TextSink) finalizeReasoning() {
	if !s.reasoningActive {
		return
	}
	// Overwrite the current \r progress line with the final count + newline
	fmt.Fprintf(s.out, "\r  ▎ thinking ··· %d chars\n", s.reasoningChars)
	s.reasoningActive = false
}

// flushReasoningProgress writes (or overwrites) the reasoning progress line.
func (s *TextSink) flushReasoningProgress() {
	if !s.reasoningActive {
		return
	}
	fmt.Fprintf(s.out, "\r  ▎ thinking ··· %d chars", s.reasoningChars)
}

// flushBatchedTools emits pending tool dispatches. When 3+, shows a summary
// line; otherwise shows one line per tool (the standard industry pattern).
func (s *TextSink) flushBatchedTools() {
	n := len(s.pendingTools)
	if n == 0 {
		return
	}
	if n >= 3 {
		fmt.Fprintf(s.out, "  ▸ %d tools running...\n", n)
	} else {
		for _, t := range s.pendingTools {
			fmt.Fprintf(s.out, "  -> %s\n", t)
		}
	}
	s.pendingTools = nil
}

// flushAggregatedErrors emits pending tool errors. When 2+, aggregates into
// a single summary line; otherwise shows the individual error.
func (s *TextSink) flushAggregatedErrors() {
	n := len(s.pendingErrors)
	if n == 0 {
		return
	}
	if n >= 2 {
		fmt.Fprintf(s.out, "  ⊘ %d tools failed: %s\n", n, strings.Join(s.pendingErrors, "; "))
	} else {
		fmt.Fprintf(s.out, "  ⊘ %s\n", s.pendingErrors[0])
	}
	s.pendingErrors = nil
}

func (s *TextSink) closeTextStream(text, reasoning string) {
	defer func() {
		s.wroteReasoningHeader = false
		s.wroteReasoningBody = false
		s.textWritten = false
	}()
	if len(text) > 0 {
		s.wroteAnything = true
	}
	if len(text) > 0 && s.renderer != nil {
		if moved := streamedRows(text, s.termWidth); moved < 200 {
			if moved == 0 {
				fmt.Fprint(s.out, "\r\033[0J")
			} else {
				fmt.Fprintf(s.out, "\r\033[%dA\033[0J", moved)
			}
			fmt.Fprint(s.out, s.renderer.Render(text))
			return
		}
	}
	if len(text) > 0 || (len(reasoning) > 0 && s.wroteReasoningBody) {
		s.out.Write(newline)
	}
}

func (s *TextSink) usageLine(u *provider.Usage, p *provider.Pricing) {
	if line := FormatUsageLine(u, p); line != "" {
		fmt.Fprintln(s.out, line)
		s.wroteAnything = true
	}
}

func FormatUsageLine(u *provider.Usage, p *provider.Pricing) string {
	if u == nil || u.TotalTokens == 0 {
		return ""
	}
	cacheCol := ""
	if u.PromptTokens > 0 {
		cached := u.CacheHitTokens
		fresh := u.CacheMissTokens
		if fresh == 0 {
			if d := u.PromptTokens - cached; d > 0 {
				fresh = d
			}
		}
		cacheCol = fmt.Sprintf(" (%d cached / %d new)", cached, fresh)
	}
	reasoning := ""
	if u.ReasoningTokens > 0 {
		reasoning = fmt.Sprintf(" (%d reasoning)", u.ReasoningTokens)
	}
	cost := ""
	if p != nil {
		cost = fmt.Sprintf(" · %s%.4f", p.Symbol(), p.Cost(u))
	}
	return fmt.Sprintf("  · %d tok · in %d%s · out %d%s%s",
		u.TotalTokens, u.PromptTokens, cacheCol, u.CompletionTokens, reasoning, cost)
}

// dimText wraps s in the ANSI dim SGR sequence. Kept for FormatUsageLine etc.
func dimText(s string) string { return "\x1b[2m" + s + "\x1b[0m" }

// writeDim writes s with ANSI dim SGR to w in three Write calls — prefix,
// content, suffix — without allocating an intermediate string.
func writeDim(w io.Writer, s string) {
	w.Write(dimPrefix)
	w.Write([]byte(s))
	w.Write(dimSuffix)
}

func CompactArgs(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 120 {
		return string(r[:120]) + "..."
	}
	return s
}
