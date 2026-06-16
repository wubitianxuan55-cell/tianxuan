// Package learning implements cross-session error pattern learning.
// It extracts recurring failure modes from tool execution results,
// stores them in .tianxuan/learned-patterns.toml, and injects
// actionable guidance into the system prompt at startup.
//
// The goal is to make tianxuan "smarter" over time: common mistakes
// (e.g. editing without first reading the file) are flagged before
// the model repeats them.
package learning

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Pattern is one recurring failure mode the system has observed across sessions.
// It records the tool, the error signature, what the model did to recover,
// and how often it has occurred.
type Pattern struct {
	Sig            string `toml:"sig"`             // SHA256(content hash), dedup key
	Tool           string `toml:"tool"`            // tool name, e.g. "edit_file"
	ErrorKind      string `toml:"error_kind"`      // normalized error class
	ErrorSnippet   string `toml:"error_snippet"`   // first 80 chars of the error message
	RecoveryAction string `toml:"recovery_action"` // what fixed it, e.g. "read file before edit"
	Count          int    `toml:"count"`           // occurrences across all sessions
	LastSeen       string `toml:"last_seen"`       // ISO 8601 date
	Skipped        bool   `toml:"skipped"`         // user has dismissed this pattern
}

// Store holds the full set of learned patterns.
type Store struct {
	Patterns []Pattern `toml:"patterns"`
}

// PatternExtractor creates a normalized pattern from a tool call observation.
// Returns nil when the call succeeded or the error is not learnable.
type PatternExtractor struct {
	// MinCount is the minimum occurrences before a pattern is surfaced.
	// Defaults to 3.
	MinCount int

	// filesPath is the .tianxuan/learned-patterns.toml path
	filesPath string
}

// toolErrorKind maps common tool errors to normalized classes.
var errorClassifiers = []struct {
	Tool    string
	MatchFn func(result string) (kind string, recovery string)
}{
	{Tool: "edit_file", MatchFn: func(r string) (string, string) {
		if strings.Contains(r, "not found") || strings.Contains(r, "old_string not found") {
			return "old_string_not_found", "read the current file content first, then search for the exact string to replace"
		}
		if strings.Contains(r, "not unique") || strings.Contains(r, "is not unique") {
			return "old_string_not_unique", "add more surrounding context to make old_string globally unique in the file"
		}
		return "", ""
	}},
	{Tool: "delete_range", MatchFn: func(r string) (string, string) {
		if strings.Contains(r, "not found") {
			return "anchor_not_found", "verify the anchor text exists in the current file content before deleting"
		}
		return "", ""
	}},
	{Tool: "delete_symbol", MatchFn: func(r string) (string, string) {
		if strings.Contains(r, "not found") {
			return "symbol_not_found", "verify the symbol name exists in the file with `grep` before deleting"
		}
		return "", ""
	}},
	{Tool: "grep", MatchFn: func(r string) (string, string) {
		if strings.Contains(r, "no results") || strings.Contains(r, "0 results") || r == "" || strings.HasPrefix(r, "no matches") {
			return "grep_no_results", "try a broader regex pattern, remove path filter, or use glob to find the file first"
		}
		return "", ""
	}},
	{Tool: "glob", MatchFn: func(r string) (string, string) {
		if strings.Contains(r, "no matches") || strings.Contains(r, "no files") || r == "" {
			return "glob_no_matches", "verify the pattern syntax; try removing ** or using ? for single characters"
		}
		return "", ""
	}},
	{Tool: "bash", MatchFn: func(r string) (string, string) {
		if strings.Contains(r, "not found") && (strings.Contains(r, "command") || strings.Contains(r, "not recognized")) {
			return "command_not_found", "install the missing tool first, or use an alternative approach"
		}
		if strings.Contains(r, "merge conflict") || strings.Contains(r, "CONFLICT") {
			return "git_merge_conflict", "resolve conflicts manually or use `git merge --abort` to cancel"
		}
		if strings.Contains(r, "timeout") || strings.Contains(r, "timed out") {
			return "bash_timeout", "reduce the operation's scope, or use run_in_background for long-running tasks"
		}
		if strings.Contains(r, "permission denied") || strings.Contains(r, "denied") {
			return "permission_denied", "check file permissions or use a different path"
		}
		if strings.Contains(r, "no such file or directory") {
			return "file_not_found", "create parent directories first or verify the path"
		}
		return "", ""
	}},
	{Tool: "web_fetch", MatchFn: func(r string) (string, string) {
		if strings.Contains(r, "timeout") || strings.Contains(r, "timed out") {
			return "fetch_timeout", "the URL may be slow or unreachable; try a different source or use web_search first"
		}
		if strings.Contains(r, "refused") || strings.Contains(r, "connection") {
			return "fetch_connection_error", "the server may be down; try a different URL or use web_search"
		}
		return "", ""
	}},
	{Tool: "write_file", MatchFn: func(r string) (string, string) {
		if strings.Contains(r, "outside") || strings.Contains(r, "confine") || strings.Contains(r, "permission") {
			return "write_outside_workspace", "the file must be within the project workspace or a configured write root"
		}
		return "", ""
	}},
}

// NewExtractor creates a pattern extractor. patternsPath is the .toml file path.
func NewExtractor(patternsPath string) *PatternExtractor {
	return &PatternExtractor{
		MinCount:  3,
		filesPath: patternsPath,
	}
}

// Extract examines a tool call result and returns a learned pattern, or nil.
func (e *PatternExtractor) Extract(toolName, result string) *Pattern {
	if result == "" || !isErrorResult(result) {
		return nil
	}

	for _, cls := range errorClassifiers {
		if cls.Tool != toolName {
			continue
		}
		kind, recovery := cls.MatchFn(result)
		if kind == "" {
			continue
		}
		return &Pattern{
			Sig:            patternSig(toolName, kind),
			Tool:           toolName,
			ErrorKind:      kind,
			ErrorSnippet:   truncate(result, 80),
			RecoveryAction: recovery,
			Count:          1,
			LastSeen:       time.Now().Format("2006-01-02"),
		}
	}
	return nil
}

// isErrorResult checks whether a tool result string indicates failure.
func isErrorResult(r string) bool {
	lower := strings.ToLower(r)
	// A tool returns error messages as plain text; successful results rarely
	// contain these keywords.
	errorIndicators := []string{
		"error:", "failed:", "not found", "not unique",
		"timeout", "denied", "refused", "conflict",
	}
	for _, ind := range errorIndicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	return false
}

// patternSig produces a stable hash for deduplication.
func patternSig(tool, kind string) string {
	h := sha256.Sum256([]byte(tool + "\x00" + kind))
	return fmt.Sprintf("%x", h[:8])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// MergePattern adds or updates a pattern in the store.
func MergePattern(store *Store, p *Pattern) {
	for i, existing := range store.Patterns {
		if existing.Sig == p.Sig {
			store.Patterns[i].Count++
			store.Patterns[i].LastSeen = p.LastSeen
			return
		}
	}
	store.Patterns = append(store.Patterns, *p)
}

// ActivePatterns returns patterns with count >= threshold, sorted by count desc.
func ActivePatterns(store *Store, minCount int) []Pattern {
	var out []Pattern
	for _, p := range store.Patterns {
		if !p.Skipped && p.Count >= minCount {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// FormatGuide formats active patterns as a system-prompt-ready guidance block.
func FormatGuide(patterns []Pattern) string {
	if len(patterns) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Learned Patterns (from past sessions)\n")
	b.WriteString("The following mistakes have occurred repeatedly. Avoid them:\n\n")
	for _, p := range patterns {
		b.WriteString(fmt.Sprintf("- %s %s: %s\n", p.Tool, p.ErrorKind, p.RecoveryAction))
	}
	return strings.TrimRight(b.String(), "\n")
}

// SaveStore persists the pattern store to disk.
func (e *PatternExtractor) SaveStore() error {
	if e.filesPath == "" {
		return nil
	}
	store, err := LoadStore(e.filesPath)
	if err != nil {
		return err
	}
	// Rebuild from disk to avoid duplication race
	return SaveStore(e.filesPath, store)
}
