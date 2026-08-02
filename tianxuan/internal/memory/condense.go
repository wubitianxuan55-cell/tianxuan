package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// pendingCondenseThreshold is the pending backlog that triggers a condensation
// pass: once more than 30 auto-extracted candidates await user confirmation,
// they are distilled into a single staged candidate so pending cannot pile up
// unboundedly (the user confirms one consolidated candidate instead of N).
const pendingCondenseThreshold = 30

// CondensePending distills a backlog of pending candidates into one staged
// candidate when their count exceeds pendingCondenseThreshold. The original
// pending files are removed because their useful content is folded into the
// single result. Returns the number of candidates staged (0 below the
// threshold or with an empty store).
func CondensePending(s Store, now time.Time) (int, error) {
	pending := PendingCandidates(s)
	if len(pending) <= pendingCondenseThreshold {
		return 0, nil
	}

	date := now.Format("2006-01-02")
	name := "condense-" + date
	if _, err := os.Stat(pendingPath(s, name)); err == nil {
		name = "condense-" + now.Format("20060102-150405")
	}

	var body strings.Builder
	body.WriteString("## 待确认记忆提炼汇总\n\n")
	var evidence []string
	for i, c := range pending {
		fmt.Fprintf(&body, "%d. **%s** — %s\n", i+1, c.Title, oneLine(c.Description))
		evidence = append(evidence, oneLine(c.Description))
	}

	cand := Candidate{
		Name:        name,
		Title:       "待确认记忆提炼 (" + date + ")",
		Description: fmt.Sprintf("%d 条待确认记忆提炼为 1 条", len(pending)),
		Type:        TypeProject,
		Body:        body.String(),
		Reason:      "pending memory condensation",
		Evidence:    strings.Join(evidence, " | "),
	}
	if err := os.MkdirAll(filepath.Join(s.Dir, pendingDir), 0o755); err != nil {
		return 0, err
	}
	b, err := json.Marshal(cand)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(pendingPath(s, name), b, 0o644); err != nil {
		return 0, err
	}

	// The originals are folded into the new candidate; removing them keeps the
	// pending list at exactly one. A removal failure surfaces loudly.
	removed := 0
	for _, c := range pending {
		if err := os.Remove(pendingPath(s, c.Name)); err != nil && !os.IsNotExist(err) {
			return 1, fmt.Errorf("condense cleanup %q: %w", c.Name, err)
		}
		removed++
	}
	return 1, nil
}
