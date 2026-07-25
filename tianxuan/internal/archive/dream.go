// Package archive — auto-dream memory extraction.
// V10.97: 蒸馏自 Bamboo Auto-Dream — 从会话归档中自动提取关键信息为记忆，
// 减少 Agent 显式调用 remember 的需要。纯规则提取，零额外 LLM 调用。
package archive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DreamConfig controls auto-dream behaviour.
type DreamConfig struct {
	// MemoryDir is where extracted memories are written (.tianxuan/memory/auto-dream/).
	MemoryDir string
	// MinMessages is the minimum session length before dreaming fires.
	MinMessages int
	// Cooldown is the minimum interval between dream runs per session.
	Cooldown time.Duration
}

// Dream extracts candidate memories from a session archive.
// It scans the JSONL records for patterns worth remembering:
//   - User preferences ("I prefer", "I like", "I usually")
//   - Project conventions ("we use", "our pattern", "convention is")
//   - Decisions ("decided to", "let's go with", "we'll use")
//
// Extracted memories are written as Markdown files to DreamConfig.MemoryDir.
// Returns the number of memories written.
func Dream(records []Record, cfg DreamConfig) int {
	if cfg.MemoryDir == "" || len(records) < cfg.MinMessages {
		return 0
	}

	var candidates []dreamCandidate

	for _, r := range records {
		if r.Role != "user" && r.Role != "assistant" {
			continue
		}
		content := r.Content
		if len(content) < 20 {
			continue
		}

		// Extract user preferences
		if prefs := extractPreferences(content); len(prefs) > 0 {
			candidates = append(candidates, prefs...)
		}
		// Extract project conventions
		if conventions := extractConventions(content); len(conventions) > 0 {
			candidates = append(candidates, conventions...)
		}
	}

	if len(candidates) == 0 {
		return 0
	}

	// Deduplicate by content hash
	seen := map[string]bool{}
	var unique []dreamCandidate
	for _, c := range candidates {
		key := strings.TrimSpace(c.body)
		if len(key) > 100 {
			key = key[:100]
		}
		if !seen[key] {
			seen[key] = true
			unique = append(unique, c)
		}
	}

	// Write memories
	if err := os.MkdirAll(cfg.MemoryDir, 0o755); err != nil {
		return 0
	}

	written := 0
	ts := time.Now().Format("2006-01-02")
	for _, c := range unique {
		fname := slugify(c.title)
		if fname == "" {
			continue
		}
		path := filepath.Join(cfg.MemoryDir, fname+".md")
		body := formatMemory(c, ts)
		if err := os.WriteFile(path, []byte(body), 0o644); err == nil {
			written++
		}
	}
	return written
}

type dreamCandidate struct {
	title string
	body  string
	kind  string // "preference", "convention", "decision"
}

// ── Pattern extraction ────────────────────────────────────────────────

var (
	prefPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:I\s+prefer|I\s+like|I\s+usually|I\s+always|I\s+tend\s+to)\s+(.{10,200}?)[.;!]`),
		regexp.MustCompile(`(?i)(?:我(?:更|比较|一般|通常|总是|习惯|喜欢|偏好))(?:是|使用|用|选)?\s*(.{10,200}?)[。；！]`),
	}
	conventionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:we\s+use|we\s+follow|our\s+(?:pattern|convention|rule|standard|approach))\s+(?:is\s+)?(.{10,200}?)[.;!]`),
		regexp.MustCompile(`(?i)(?:convention(?:\s+is)?|the\s+pattern\s+is|the\s+rule\s+is)\s+(.{10,200}?)[.;!]`),
		regexp.MustCompile(`(?i)(?:我们(?:用|使用|遵循|采用))(?:的)?\s*(.{10,200}?)[。；！]`),
	}
)

func extractPreferences(content string) []dreamCandidate {
	var results []dreamCandidate
	for _, re := range prefPatterns {
		matches := re.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if len(m) > 1 {
				body := strings.TrimSpace(m[1])
				if len(body) > 20 {
					results = append(results, dreamCandidate{
						title: truncateTitle(body, 60),
						body:  body,
						kind:  "preference",
					})
				}
			}
		}
	}
	return results
}

func extractConventions(content string) []dreamCandidate {
	var results []dreamCandidate
	for _, re := range conventionPatterns {
		matches := re.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if len(m) > 1 {
				body := strings.TrimSpace(m[1])
				if len(body) > 20 {
					results = append(results, dreamCandidate{
						title: truncateTitle(body, 60),
						body:  body,
						kind:  "convention",
					})
				}
			}
		}
	}
	return results
}

func truncateTitle(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	s = strings.Trim(s, "-")
	// Remove consecutive dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) > 50 {
		s = s[:50]
	}
	return s
}

func formatMemory(c dreamCandidate, ts string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: " + c.kind + "\n")
	b.WriteString("source: auto-dream\n")
	b.WriteString("date: " + ts + "\n")
	b.WriteString("---\n\n")
	b.WriteString("# " + c.title + "\n\n")
	b.WriteString(c.body + "\n")
	return b.String()
}

// ── Batch dream runner ─────────────────────────────────────────────────

// DreamBatch runs dream extraction over all sessions in an archive directory,
// respecting per-session cooldown via a last-dreamed timestamp file.
func DreamBatch(archiveDir, memoryDir string, minMessages int, cooldown time.Duration) int {
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return 0
	}

	total := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(archiveDir, entry.Name())
		records, err := readArchive(path)
		if err != nil || len(records) < minMessages {
			continue
		}

		// Cooldown check: skip if dreamed recently
		stampFile := path + ".dreamed"
		if cooldown > 0 {
			if info, err := os.Stat(stampFile); err == nil {
				if time.Since(info.ModTime()) < cooldown {
					continue
				}
			}
		}

		cfg := DreamConfig{
			MemoryDir:   memoryDir,
			MinMessages: minMessages,
			Cooldown:    cooldown,
		}
		n := Dream(records, cfg)
		if n > 0 {
			// Touch stamp file
			os.WriteFile(stampFile, nil, 0o644)
		}
		total += n
	}
	return total
}

// readArchive reads all records from a JSONL archive file.
func readArchive(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []Record
	dec := json.NewDecoder(f)
	for dec.More() {
		var r Record
		if err := dec.Decode(&r); err != nil {
			// Skip malformed lines (archive is best-effort)
			continue
		}
		records = append(records, r)
	}
	return records, nil
}
