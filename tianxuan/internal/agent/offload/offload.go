// Package offload implements context offloading for large tool outputs.
// When a tool returns output exceeding the configured threshold, the full
// output is saved to the filesystem and the model receives only a compact
// reference (path + preview). This prevents context window saturation from
// large tool results while keeping the data accessible on demand via the
// search_large_output tool.
//
// Design distilled from manishiitg/mcpagent's Context Offloading:
//
//	https://github.com/manishiitg/mcpagent
//
// The "offload context" pattern is one of three primary context engineering
// strategies (Manus): offload, reduce, isolate.
package offload

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultThresholdChars is the default output size (in Unicode characters)
// above which the result is offloaded to disk.
const DefaultThresholdChars = 10000

// PreviewChars is the number of leading characters kept in the model-visible
// reference so the agent can judge relevance before calling search_large_output.
const PreviewChars = 200

// Store manages offloaded tool outputs for one session.
type Store struct {
	mu   sync.Mutex
	dir  string
	seq  int
}

// NewStore creates an offload store rooted at baseDir/<sessionID>/.
// Caller is responsible for ensuring baseDir exists.
func NewStore(baseDir, sessionID string) (*Store, error) {
	dir := filepath.Join(baseDir, sanitizeSessionID(sessionID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("offload: create dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the store directory path.
func (s *Store) Dir() string { return s.dir }

// MaybeOffload checks whether raw output exceeds the threshold. If it does,
// the full output is saved and a compact reference string is returned.
// Otherwise raw is returned unchanged.
func (s *Store) MaybeOffload(toolName string, raw string, thresholdChars int) string {
	if thresholdChars <= 0 {
		thresholdChars = DefaultThresholdChars
	}
	if charCount(raw) <= thresholdChars {
		return raw
	}
	ref, err := s.offload(toolName, raw)
	if err != nil {
		// Degrade gracefully: return the full output if offload fails.
		return raw
	}
	return ref
}

func (s *Store) offload(toolName string, raw string) (string, error) {
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	ts := time.Now().Unix()
	name := fmt.Sprintf("%s-%d-%d.txt", sanitizeToolName(toolName), ts, seq)
	path := filepath.Join(s.dir, name)

	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		return "", fmt.Errorf("offload: write: %w", err)
	}

	preview := raw
	if len(preview) > PreviewChars {
		preview = preview[:PreviewChars]
	}
	// Collapse whitespace in preview for compactness.
	preview = strings.TrimSpace(preview)

	return fmt.Sprintf(
		"[Large output offloaded: %s (%d chars, %d bytes)]\nPreview: %s...\nUse search_large_output to read, search, or query this file.",
		path,
		charCount(raw),
		len(raw),
		preview,
	), nil
}

// FileInfo describes one offloaded file for search_large_output.
type FileInfo struct {
	Name    string
	Path    string
	Size    int64
	Tool    string
	ModTime time.Time
}

// List returns metadata for all offloaded files in this store, newest first.
func (s *Store) List() ([]FileInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []FileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    e.Name(),
			Path:    filepath.Join(s.dir, e.Name()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	// Reverse: newest first (os.ReadDir returns alpha order).
	for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
		files[i], files[j] = files[j], files[i]
	}
	return files, nil
}

// Read returns the full content of an offloaded file by its base name
// (e.g. "bash-1700000000-1.txt").
func (s *Store) Read(name string) (string, error) {
	// Path traversal guard.
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("offload: invalid name %q", name)
	}
	path := filepath.Join(s.dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Search performs a case-insensitive substring search across all offloaded
// files and returns matching lines with file context.
func (s *Store) Search(query string, maxHits int) (string, error) {
	if maxHits <= 0 {
		maxHits = 20
	}
	files, err := s.List()
	if err != nil {
		return "", err
	}
	lower := strings.ToLower(query)
	var out strings.Builder
	hits := 0
	for _, f := range files {
		if hits >= maxHits {
			break
		}
		content, err := s.Read(f.Name)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(content, "\n") {
			if hits >= maxHits {
				break
			}
			if strings.Contains(strings.ToLower(line), lower) {
				fmt.Fprintf(&out, "%s:%d: %s\n", f.Name, i+1, line)
				hits++
			}
		}
	}
	if out.Len() == 0 {
		out.WriteString("(no matches)")
	}
	return out.String(), nil
}

// RemoveAll deletes all offloaded files for this store (called at session end).
func (s *Store) RemoveAll() error {
	return os.RemoveAll(s.dir)
}

// ── helpers ──────────────────────────────────────────────────────

func charCount(s string) int {
	return len([]rune(s))
}

func sanitizeSessionID(id string) string {
	// Keep alphanumeric, hyphen, underscore; replace others.
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, id)
}

func sanitizeToolName(name string) string {
	// Replace MCP namespace separators and special chars with hyphen.
	name = strings.ReplaceAll(name, "__", "-")
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
}

// ConfigFingerprint computes a short hash of the offload config for cache
// invalidation when settings change.
func ConfigFingerprint(baseDir string, thresholdChars int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", baseDir, thresholdChars)))
	return hex.EncodeToString(h[:])[:8]
}
