package agent

import (
	"fmt"
	"strings"

	"tianxuan/internal/ccr"
	"tianxuan/internal/provider"
)

// PruneStats reports one prune pass.
type PruneStats struct {
	Results    int
	SavedChars int
	Archive    string
}

const (
	prunedMarker = "[pruned ccr:"
	minPruneBytes = 1024
)

// PruneStaleToolResults elides tool-result content older than the protected
// recent tail. Originals are stored in CCR so the LLM can retrieve them on
// demand via the `retrieve` tool — no need to re-run the command.
// V9.1: CCR marker replaces old "re-run the tool" instruction.
func (a *AgentRunner) PruneStaleToolResults() (PruneStats, error) {
	var st PruneStats
	if a.compaction.Window <= 0 {
		return st, nil
	}
	msgs := a.session.Messages
	head, start, ok := a.planCompaction(msgs, 1)
	if !ok {
		return st, nil
	}
	var idx []int
	for i := head; i < start; i++ {
		m := msgs[i]
		if m.Role != provider.RoleTool || len(m.Content) < minPruneBytes || strings.HasPrefix(m.Content, prunedMarker) {
			continue
		}
		idx = append(idx, i)
	}
	if len(idx) == 0 {
		return st, nil
	}
	if a.compaction.ArchiveDir != "" {
		originals := make([]provider.Message, 0, len(idx))
		for _, i := range idx {
			originals = append(originals, msgs[i])
		}
		path, err := archiveMessages(a.compaction.ArchiveDir, originals)
		if err != nil {
			return st, fmt.Errorf("archive: %w", err)
		}
		st.Archive = path
	}
	next := append([]provider.Message(nil), msgs...)
	for _, i := range idx {
		m := next[i]
		key := ccr.Write(m.Content)
		summary := summarizeCCRContent(m.Content)
		placeholder := fmt.Sprintf("%s%s, %s · %d bytes, use retrieve(key=%q) to get full output]",
			prunedMarker, m.Name, summary, len(m.Content), key)
		st.SavedChars += len(m.Content) - len(placeholder)
		m.Content = placeholder
		next[i] = m
		st.Results++
	}
	a.session.Replace(next)
	a.session.IncrementRewrite()
	return st, nil
}

// summarizeCCRContent extracts a short signal summary from tool output.
func summarizeCCRContent(s string) string {
	lines := strings.Split(s, "\n")
	signals := make([]string, 0, 3)
	for _, line := range lines {
		for _, kw := range []string{"FATAL", "ERROR", "FAIL", "panic", "PASS", "ok", "exit status"} {
			if strings.Contains(line, kw) {
				signals = append(signals, strings.TrimSpace(line))
				break
			}
		}
		if len(signals) >= 3 {
			break
		}
	}
	if len(signals) > 0 {
		return strings.Join(signals, " | ")
	}
	// Fallback: first meaningful line
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if len(t) > 5 && !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "//") {
			if len(t) > 100 {
				t = t[:100]
			}
			return t
		}
	}
	return fmt.Sprintf("%d bytes", len(s))
}
