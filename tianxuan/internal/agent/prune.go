package agent

import (
	"fmt"
	"strings"

	"tianxuan/internal/provider"
)

// PruneStats reports one prune pass.
type PruneStats struct {
	Results    int
	SavedChars int
	Archive    string
}

const (
	prunedMarker  = "[elided tool result — "
	minPruneBytes = 1024
)

// PruneStaleToolResults elides tool-result content older than the protected
// recent tail, archiving the originals first. Idempotent; a no-op when
// compaction is disabled (no context window).
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
		placeholder := fmt.Sprintf("%s%s, %d bytes dropped to save context; re-run the tool if the data is needed again]", prunedMarker, m.Name, len(m.Content))
		st.SavedChars += len(m.Content) - len(placeholder)
		m.Content = placeholder
		next[i] = m
		st.Results++
	}
	a.session.Replace(next)
	a.session.IncrementRewrite()
	return st, nil
}
