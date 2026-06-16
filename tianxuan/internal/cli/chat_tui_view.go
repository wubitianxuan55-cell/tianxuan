package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"tianxuan/internal/i18n"
	"tianxuan/internal/tool"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m chatTUI) View() tea.View {
	boxW := m.width
	if boxW < 10 {
		boxW = 10
	}
	box := inputBoxStyle.Width(boxW).Render(m.input.View())

	var modeTag string
	switch {
	case m.ctrl.Bypass():
		modeTag = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("[YOLO]")
	case m.planMode:
		modeTag = yellow("[plan]")
	default:
		modeTag = dim("[auto]")
	}

	ctxTag := m.contextTag()
	var status string
	switch {
	case m.rewind != nil:
		status = "  " + modeTag + " · ⟲ rewind"
	case m.chooser != nil:
		status = "  " + modeTag + " · " + i18n.M.ChatStatusQuestion
	case m.pendingApproval != nil && m.pendingApproval.Tool == planApprovalTool:
		status = "  " + modeTag + " · " + i18n.M.ChatStatusPlanApproval
	case m.pendingApproval != nil:
		status = "  " + modeTag + " · " + i18n.M.ChatStatusToolApproval
	case m.state == tuiRunning:
		status = fmt.Sprintf("  %s · "+i18n.M.ChatStatusThinkingFmt, modeTag, m.spinner.View(), m.elapsed)
		if m.turnTokens > 0 {
			status += " · ↓" + shortTokens(m.turnTokens)
		}
	default:
		status = "  " + modeTag + " · " + i18n.M.ChatStatusIdle
	}
	// Second status row: the live data (context gauge, cache rates, jobs,
	// balance). It lives on its own fixed row so it's always shown in full rather
	// than being truncated off the end of the keybinding hints on line 1. Two
	// rows is a fixed height, so unlike a wrap-when-long status it doesn't
	// reintroduce resize ghosting.
	var data []string
	if ctxTag != "" {
		data = append(data, ctxTag)
	}
	if cache := m.cacheTag(); cache != "" {
		data = append(data, cache)
	}
	if jt := m.jobsTag(); jt != "" {
		data = append(data, jt)
	}
	if m.balance != "" {
		data = append(data, dim(m.balance))
	}
	dataLine := "  " + strings.Join(data, " · ")
	// A configured custom status line replaces the built-in data row entirely.
	if m.statuslineCmd != "" && m.statuslineOut != "" {
		dataLine = "  " + m.statuslineOut
	}

	// Bottom region pinned under the transcript viewport: optional panels, the
	// input box, then the two status rows. Its height feeds transcriptHeight so
	// the viewport above fills exactly the rest of the screen.
	var parts []string
	rowsAboveBox := 0 // terminal rows occupied by todo/banner/menu before the input box
	if todo := m.renderTodoPanel(); todo != "" {
		parts = append(parts, todo)
		rowsAboveBox += strings.Count(todo, "\n") + 1
	}
	if banner := m.renderApprovalBanner(); banner != "" {
		parts = append(parts, banner)
		rowsAboveBox += strings.Count(banner, "\n") + 1
	}
	if card := m.renderChooser(); card != "" {
		parts = append(parts, card)
		rowsAboveBox += strings.Count(card, "\n") + 1
	}
	if card := m.renderRewind(); card != "" {
		parts = append(parts, card)
		rowsAboveBox += strings.Count(card, "\n") + 1
	}
	if tray := m.renderAttachmentTray(); tray != "" {
		parts = append(parts, tray)
		rowsAboveBox += strings.Count(tray, "\n") + 1
	}
	if menu := m.renderCompletion(); menu != "" {
		parts = append(parts, menu)
		rowsAboveBox += strings.Count(menu, "\n") + 1
	}
	// Fixed two-row status: line 1 = mode + keybinding/state hints, line 2 = live
	// data. Each row is clamped to width independently so neither wraps.
	statusBlock := clampStatusLine(status, boxW) + "\n" + clampStatusLine(dataLine, boxW)
	// Pad to the full width so the status rows overwrite the whole line — an
	// unpadded (short) status leaves stale cells from the prior frame on the
	// right (alt-screen only writes the cells the frame actually contains).
	parts = append(parts, box, statusStyle.Width(boxW).MaxWidth(boxW).Render(statusBlock))

	// Full-screen frame: the transcript viewport on top (it pads to exactly its
	// height), the pinned bottom region beneath. Alt-screen owns the grid, so
	// resize repaints cleanly — no scrollback reflow, no ghost borders.
	v := tea.NewView(m.renderTranscript() + "\n" + strings.Join(parts, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion // wheel scrolls the transcript
	// Anchor the real terminal cursor at the textarea's insertion point so IME
	// candidate windows appear in the input box. input.Cursor() is relative to
	// the textarea; offset by the viewport height + rows above + the box's top
	// border row (+1 column for PaddingLeft).
	if cur := m.input.Cursor(); cur != nil {
		cur.X += 1
		cur.Y += m.viewport.Height() + rowsAboveBox + 1
		v.Cursor = cur
	}
	return v
}
func (m chatTUI) contextTag() string {
	used, window := m.ctrl.ContextSnapshot()
	if used == 0 || window == 0 {
		return ""
	}
	pct := used * 100 / window
	ratio := m.ctrl.CompactRatio()
	if ratio <= 0 || ratio >= 1 {
		// Compaction disabled: just the raw gauge, coloured on window fill.
		body := fmt.Sprintf("%s / %s ctx (%d%%)", shortTokens(used), shortTokens(window), pct)
		switch {
		case pct >= 85:
			return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(body)
		case pct >= 60:
			return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(body)
		default:
			return dim(body)
		}
	}
	threshold := int(ratio * 100)
	// Headroom to the compaction point, as a percentage of the window (clamped at 0).
	left := threshold - pct
	if left < 0 {
		left = 0
	}
	body := fmt.Sprintf("%s ctx (%d%%) · %d%% to compact", shortTokens(used), pct, left)
	switch {
	case pct >= threshold:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(fmt.Sprintf("%s ctx (%d%%) · compacting soon", shortTokens(used), pct))
	case left <= 10:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(body)
	default:
		return dim(body)
	}
}
func (m chatTUI) cacheTag() string {
	now := ""
	if u := m.ctrl.LastUsage(); u != nil {
		d := u.CacheHitTokens + u.CacheMissTokens
		if d == 0 {
			d = u.PromptTokens
		}
		if d > 0 {
			now = fmt.Sprintf("cache %d%%", u.CacheHitTokens*100/d)
		}
	}
	avg := ""
	if hit, miss := m.ctrl.SessionCache(); hit+miss > 0 {
		avg = fmt.Sprintf("avg %d%%", hit*100/(hit+miss))
	}
	switch {
	case now != "" && avg != "":
		return dim(now + " · " + avg)
	case now != "":
		return dim(now)
	case avg != "":
		return dim(avg)
	}
	return ""
}
func (m chatTUI) jobsTag() string {
	n := len(m.ctrl.Jobs())
	if n == 0 {
		return ""
	}
	return dim(fmt.Sprintf("⚙ %d", n))
}
func shortTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dK", n/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
func (m chatTUI) renderApprovalBanner() string {
	w := m.width
	if w < 10 {
		w = 10
	}
	if m.pendingApproval == nil {
		return ""
	}
	// A plan approval shows the gate prompt (the plan itself is already printed as
	// the assistant's reply); a tool approval names the tool + subject.
	if m.pendingApproval.Tool == planApprovalTool {
		return approvalBannerStyle.Width(w).Render("⏸ " + i18n.M.PlanApprovalPrompt)
	}
	name, detail := approvalToolDetails(m.pendingApproval.Tool)
	subj := strings.TrimSpace(m.pendingApproval.Subject)
	if subj != "" {
		subj = " " + truncateSubject(subj, w)
	}
	text := fmt.Sprintf(i18n.M.ToolApprovalPromptFmt, name, subj, detail)
	return approvalBannerStyle.Width(w).Render("⏸ " + text)
}
func approvalToolDetails(toolName string) (name, detail string) {
	if server, short, ok := tool.SplitMCPName(toolName); ok {
		lines := []string{}
		if strings.EqualFold(short, "understand_image") {
			lines = append(lines, i18n.M.ToolApprovalImageUse)
		}
		lines = append(lines, fmt.Sprintf(i18n.M.ToolApprovalSourceFmt, server))
		return short, strings.Join(lines, "\n")
	}
	return toolName, fmt.Sprintf(i18n.M.ToolApprovalSourceFmt, i18n.M.ToolApprovalBuiltIn)
}
func (m chatTUI) renderTodoPanel() string {
	var p struct {
		Todos []struct {
			Content    string `json:"content"`
			Status     string `json:"status"`
			ActiveForm string `json:"activeForm"`
			Level      int    `json:"level"`
		} `json:"todos"`
	}
	if err := json.Unmarshal([]byte(m.todoArgs), &p); err != nil || len(p.Todos) == 0 {
		return ""
	}
	done := 0
	for _, t := range p.Todos {
		if t.Status == "completed" {
			done++
		}
	}
	if done == len(p.Todos) {
		return "" // all finished — clear the panel
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", accent("To-dos"), dim(fmt.Sprintf("%d/%d", done, len(p.Todos))))
	shown := 0
	for _, t := range p.Todos {
		if shown >= todoPanelMaxRows {
			b.WriteString(dim(fmt.Sprintf("  +%d more", len(p.Todos)-shown)) + "\n")
			break
		}
		shown++
		indent := "  "
		if t.Level >= 1 {
			indent = "      " // sub-steps sit under their phase
		}
		switch t.Status {
		case "completed":
			b.WriteString(indent + green("✔") + " " + dim(t.Content) + "\n")
		case "in_progress":
			label := t.Content
			if t.ActiveForm != "" {
				label = t.ActiveForm
			}
			b.WriteString(indent + yellow("▶ "+label) + "\n")
		default:
			b.WriteString(indent + dim("○ "+t.Content) + "\n")
		}
	}
	return todoPanelStyle.Width(max(m.width, 10)).Render(strings.TrimRight(b.String(), "\n"))
}
func (m chatTUI) renderAttachmentTray() string {
	if len(m.attachments) == 0 {
		return ""
	}
	labels := attachmentLabels(m.attachments)
	body := strings.Join(labels, "  ")
	return todoPanelStyle.Width(max(m.width, 10)).Render(body)
}
func truncateSubject(s string, width int) string {
	max := width - 28
	if max < 16 {
		max = 16
	}
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
func clampStatusLine(s string, width int) string {
	// ansi.Truncate is ANSI-aware, counts wide chars, and appends the tail when
	// it actually clips — one row regardless of how many tags the status carries.
	return ansi.Truncate(s, width, "…")
}