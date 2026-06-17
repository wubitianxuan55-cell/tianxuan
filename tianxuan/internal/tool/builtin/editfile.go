package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(editFile{}) }

// editFile replaces an exact string in a file. roots confines the target to the
// workspace when non-empty (see writeFile); workDir, when non-empty, is the
// directory a relative path resolves against (see resolveIn).
type editFile struct {
	roots   []string
	workDir string
}

func (editFile) Name() string { return "edit_file" }

func (editFile) Description() string {
	return "Replace an exact string in a file with another. old_string must occur exactly once; add surrounding context to disambiguate. Use for targeted edits instead of rewriting the whole file."
}

func (editFile) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path"},"old_string":{"type":"string","description":"Exact text to replace (must be unique in the file)"},"new_string":{"type":"string","description":"Replacement text (may be empty to delete)"}},"required":["path","old_string","new_string"]}`)
}

func (editFile) ReadOnly() bool { return false }

func (editFile) CompactDescription() string { return compactDesc["edit_file"] }
func (editFile) CompactSchema() json.RawMessage   { return compactSchema["edit_file"] }

func (e editFile) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	if p.OldString == "" {
		return "", fmt.Errorf("old_string is required")
	}
	p.Path = resolveIn(e.workDir, p.Path)
	if err := confine(e.roots, p.Path); err != nil {
		return "", err
	}

	b, err := os.ReadFile(p.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}
	content := string(b)

	switch c := strings.Count(content, p.OldString); c {
	case 0:
		// Fuzzy: find the closest matches so the model can retry without a read_file round-trip.
		candidates := findSimilar(content, p.OldString, 3)
		if len(candidates) == 0 {
			return "", fmt.Errorf("old_string not found in %s", p.Path)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "old_string not found in %s. Closest matches:\n", p.Path)
		for i, cand := range candidates {
			fmt.Fprintf(&b, "\n  candidate %d (line %d-%d, score %d%%):\n", i+1, cand.StartLine, cand.EndLine, cand.Score)
			for _, line := range cand.Lines {
				fmt.Fprintf(&b, "    %s\n", line)
			}
		}
		b.WriteString("\nChoose a candidate number and retry with its exact text as old_string.")
		return b.String(), nil
	case 1:
		// ok
	default:
		return "", fmt.Errorf("old_string is not unique in %s; add more surrounding context", p.Path)
	}

	updated := strings.Replace(content, p.OldString, p.NewString, 1)
	if err := os.WriteFile(p.Path, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", p.Path, err)
	}
	return fmt.Sprintf("edited %s", p.Path), nil
}

// similarCandidate is a region of the file that resembles old_string.
type similarCandidate struct {
	StartLine int
	EndLine   int
	Lines     []string
	Score     int // 0-100
}

// findSimilar scans content for the top-n regions most similar to query.
// It tokenises query into words, scores each line by word overlap, and groups
// consecutive scored lines into candidates.
func findSimilar(content string, query string, n int) []similarCandidate {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil
	}
	// Tokenise query into words (min 2 chars).
	queryWords := make(map[string]bool)
	for _, w := range strings.Fields(query) {
		if len(w) >= 2 {
			queryWords[w] = true
		}
	}
	if len(queryWords) == 0 {
		return nil
	}

	// Score each line by word overlap.
	scores := make([]int, len(lines))
	totalWords := len(queryWords)
	maxScore := 0
	for i, line := range lines {
		seen := make(map[string]bool)
		for _, w := range strings.Fields(line) {
			if queryWords[w] && !seen[w] {
				seen[w] = true
				scores[i]++
			}
		}
		if scores[i] > maxScore {
			maxScore = scores[i]
		}
	}
	if maxScore == 0 {
		return nil
	}

	// Group consecutive lines with score > 0 into candidates.
	type rawCand struct {
		start, end int
		score      int
	}
	var raw []rawCand
	for i := 0; i < len(lines); {
		if scores[i] == 0 {
			i++
			continue
		}
		start := i
		score := 0
		for i < len(lines) && scores[i] > 0 {
			score += scores[i]
			i++
		}
		raw = append(raw, rawCand{start, i - 1, score})
	}

	// Sort by score descending, take top n.
	// Simple selection: keep top n in a min-heap fashion.
	type scored struct {
		idx   int
		score int
	}
	top := make([]scored, 0, n)
	for i, c := range raw {
		// Normalise to 0-100.
		normalised := c.score * 100 / (totalWords * (c.end - c.start + 1))
		if normalised > 100 {
			normalised = 100
		}
		if len(top) < n {
			top = append(top, scored{i, normalised})
		} else {
			// Replace lowest.
			worst := 0
			for j := 1; j < len(top); j++ {
				if top[j].score < top[worst].score {
					worst = j
				}
			}
			if normalised > top[worst].score {
				top[worst] = scored{i, normalised}
			}
		}
	}

	// Build result in original file order.
	byStart := make(map[int]scored)
	for _, s := range top {
		byStart[raw[s.idx].start] = s
	}
	var out []similarCandidate
	for i, c := range raw {
		if s, ok := byStart[c.start]; ok {
			out = append(out, similarCandidate{
				StartLine: c.start + 1,
				EndLine:   c.end + 1,
				Lines:     lines[c.start : c.end+1],
				Score:     s.score,
			})
			_ = i
			if len(out) >= len(top) {
				break
			}
		}
	}
	return out
}
