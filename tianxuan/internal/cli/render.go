package cli

import (
	"image/color"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"tianxuan/internal/agent"
	"tianxuan/internal/event"
	"tianxuan/internal/i18n"
	"tianxuan/internal/provider"
)

// replaySectionsFor turns a loaded session into scrollback blocks: user bubbles
// and assistant markdown. Tool messages are dropped — needed in session state
// but noise in the visible transcript on resume.
func replaySectionsFor(history []provider.Message, width int, renderer *mdRenderer) []string {
	var out []string
	for _, m := range history {
		switch m.Role {
		case provider.RoleUser:
			content := agent.StripTransientBlocks(m.Content)
			if strings.HasPrefix(strings.TrimSpace(content), "<compaction-summary>") {
				content = "〈会话摘要〉"
			}
			out = append(out, renderUserBubble(content, width)+"\n\n")
		case provider.RoleAssistant:
			body := strings.TrimSpace(m.Content)
			if body == "" {
				continue
			}
			rendered := renderer.Render(body)
			if rendered == "" {
				rendered = body
			}
			out = append(out, rendered+"\n")
		}
	}
	return out
}

// renderTUIBanner is the title + tip + optional missing-key warning printed once
// at the top of the session.
func renderTUIBanner(label, missing string, width int) string {
	var b strings.Builder
	b.WriteString(accent("◆") + " " + bold("tianxuan chat") + "  " + dim("· "+label) + "\n")
	b.WriteString(dim("  "+i18n.M.ChatTip) + "\n")
	if missing != "" {
		b.WriteString(wrapForViewport("  ! "+missing, width, lipgloss.Color("3")) + "\n")
	}
	return b.String()
}

// wrapForViewport hard-wraps text to fit width columns and colours every line.
func wrapForViewport(text string, width int, fg color.Color) string {
	if width <= 0 {
		width = 80
	}
	return lipgloss.NewStyle().
		Foreground(fg).
		Width(width).
		Render(text)
}

// renderUserBubble styles the just-submitted line with a filled dim background.
func renderUserBubble(line string, width int) string {
	line = displayLineForImageRefs(line)
	prefix := "› "
	if !colorEnabled {
		return "│ " + prefix + line
	}
	w := width - 4
	if w < 10 {
		w = 10
	}
	bubble := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Width(w).
		Padding(0, 1)
	return bubble.Render(prefix + line)
}

var cliImageRefRe = regexp.MustCompile(`(?:^|\s)@\.tianxuan/attachments/clipboard-\d{8}-\d{6}\.\d+(?:-(?:\d{6}|[a-f0-9]{8}))?\.(?:png|jpg|jpeg|gif|webp)`)

func displayLineForImageRefs(line string) string {
	idx := 0
	out := cliImageRefRe.ReplaceAllStringFunc(line, func(_ string) string {
		idx++
		return " [image" + strconv.Itoa(idx) + "]"
	})
	return strings.TrimSpace(out)
}

// eventSink is the event.Sink the agent emits to in TUI mode. Each event
// becomes an agentEventMsg. The channel is generously buffered so streaming
// bursts don't back-pressure the agent goroutine.
type eventSink struct {
	ch chan<- event.Event
}

func (s *eventSink) Emit(e event.Event) { s.ch <- e }

// compactArgs delegates to agent.CompactArgs so the CLI and headless rendering
// stay identical.
func compactArgs(s string) string { return agent.CompactArgs(s) }
