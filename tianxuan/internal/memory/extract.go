package memory

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tianxuan/internal/provider"
)

// extractCursorFile tracks how far auto-extract has scanned in the current
// session, mirroring Qwen Code's extract-cursor semantics: the processed
// offset prevents re-extracting the same messages, and a session change
// restarts scanning (history may differ after a resume).
const extractCursorFile = "extract-cursor.json"

// pendingDir is where auto-extracted candidates wait for user confirmation
// before they are written into active memory.
const pendingDir = "pending"

// ExtractCursor is the persisted auto-extract position for one session.
type ExtractCursor struct {
	SessionID       string `json:"sessionId"`
	ProcessedOffset int    `json:"processedOffset"`
}

// Candidate is a memory staged by auto-extract, awaiting user confirmation.
type Candidate struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        Type   `json:"type"`
	Body        string `json:"body"`
	Reason      string `json:"reason"`
	Evidence    string `json:"evidence"`
}

// readExtractCursor loads the persisted cursor; a missing file is a zero
// cursor (scan from the start).
func readExtractCursor(s Store) ExtractCursor {
	b, err := os.ReadFile(filepath.Join(s.Dir, extractCursorFile))
	if err != nil {
		return ExtractCursor{}
	}
	var c ExtractCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return ExtractCursor{}
	}
	return c
}

func writeExtractCursor(s Store, c ExtractCursor) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, extractCursorFile), b, 0o644)
}

// pendingPath returns the staging file for one candidate.
func pendingPath(s Store, name string) string {
	return filepath.Join(s.Dir, pendingDir, slug(name)+".json")
}

// isTransientBlock reports whether a message is an auto-injected control block
// rather than real user/assistant content — such blocks must never become
// memory candidates. Angle-bracket blocks (<memory-update>, <session-facts>,
// <background-jobs>) are host-injected and matched anywhere in the message —
// the host can append them after the user's own text, so a prefix-only check
// let them leak into candidates. Bracket blocks ([auto-recall], [system]) keep
// prefix semantics because those tokens can legitimately appear mid-message in
// user prose.
func isTransientBlock(content string) bool {
	text := strings.TrimSpace(content)
	for _, tag := range []string{"<memory-update>", "<session-facts>", "<background-jobs>"} {
		if strings.Contains(text, tag) {
			return true
		}
	}
	for _, prefix := range []string{"[auto-recall]", "[system]"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// memoryMarkers are the durable-statement triggers shared with the desktop
// suggestion extractor: an explicit remember request or a future-facing
// preference/rule marker.
var memoryMarkers = []struct{ value, reason string }{
	{"记住", "explicit remember request"},
	{"以后", "future-facing preference"},
	{"始终", "persistent working rule"},
	{"总是", "persistent working rule"},
	{"每次", "repeated workflow preference"},
	{"默认", "default behavior preference"},
	{"不要", "negative working preference"},
	{"偏好", "user preference"},
	{"规则", "durable rule"},
	{"约定", "project convention"},
	{"remember", "explicit remember request"},
	{"always", "persistent working rule"},
	{"never", "negative working preference"},
	{"prefer", "user preference"},
	{"preference", "user preference"},
	{"by default", "default behavior preference"},
}

const maxMemoryStatementChars = 420

// extractMemoryStatement pulls a durable statement out of a message using the
// shared marker set, returning (statement, reason) or ("", "") when nothing
// durable is present.
func extractMemoryStatement(content string) (string, string) {
	text := oneLine(content)
	if len([]rune(text)) < 8 || len([]rune(text)) > maxMemoryStatementChars {
		return "", ""
	}
	lower := strings.ToLower(text)
	for _, m := range memoryMarkers {
		if strings.Contains(lower, m.value) {
			return trimMemoryLead(text, m.value), m.reason
		}
	}
	return "", ""
}

func trimMemoryLead(text, marker string) string {
	idx := strings.Index(strings.ToLower(text), marker)
	if idx < 0 {
		return text
	}
	trimmed := strings.TrimSpace(text[idx:])
	for _, sep := range []string{"：", ":", "-", "—"} {
		trimmed = strings.TrimPrefix(trimmed, marker+sep)
	}
	return strings.TrimSpace(trimmed)
}

// inferMemoryType classifies a candidate by its content, mirroring the desktop
// extractor's heuristics.
func inferMemoryType(statement string) Type {
	lower := strings.ToLower(statement)
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "github.com/") {
		return TypeReference
	}
	if hasAny(lower, "反馈", "回复", "回答", "不要", "always", "never", "始终", "总是") {
		return TypeFeedback
	}
	if hasAny(lower, "项目", "分支", "pr", "pull request", "仓库", "repo", "约定") {
		return TypeProject
	}
	return TypeUser
}

func candidateName(desc string) string {
	if n := slug(desc); n != "" {
		return n
	}
	sum := sha256.Sum256([]byte(desc))
	return fmt.Sprintf("candidate-%x", sum[:4])
}

func candidateTitle(desc string) string {
	if t := truncateRunes(oneLine(desc), 64); t != "" {
		return t
	}
	return "Memory candidate"
}

// existingMemoryKeys returns normalized text of every active memory, used to
// skip candidates already covered.
func existingMemoryKeys(store Store) []string {
	var out []string
	for _, m := range store.List() {
		out = append(out, normalizeKey(strings.Join([]string{m.Name, m.Title, m.Description, m.Body}, " ")))
	}
	return out
}

// ExtractCandidates scans the unprocessed tail of a session's message log for
// durable statements and stages them under <store>/pending/ for user
// confirmation. It advances the extract cursor so a later call only scans new
// messages. Returns the number of newly staged candidates.
func ExtractCandidates(s Store, sessionID string, msgs []provider.Message) (int, error) {
	if s.Dir == "" {
		return 0, nil
	}
	cursor := readExtractCursor(s)
	start := 0
	if cursor.SessionID == sessionID {
		start = cursor.ProcessedOffset
	}
	// History may shrink between calls (compaction): clamp so new messages
	// are never skipped after a rewrite.
	if start > len(msgs) || start < 0 {
		start = 0
	}
	slice := msgs[start:]

	hasNewContent := false
	for _, m := range slice {
		if m.Role == provider.RoleUser || m.Role == provider.RoleAssistant {
			if strings.TrimSpace(m.Content) != "" && !isTransientBlock(m.Content) {
				hasNewContent = true
				break
			}
		}
	}
	if !hasNewContent {
		return 0, writeExtractCursor(s, ExtractCursor{SessionID: sessionID, ProcessedOffset: len(msgs)})
	}

	if err := os.MkdirAll(filepath.Join(s.Dir, pendingDir), 0o755); err != nil {
		return 0, err
	}
	// Dedup baseline: active memory plus already-staged pending candidates. A
	// later session re-stating the same rule must not stack duplicate pending
	// files — without this, repeated workflows accumulated go-build style
	// duplicates in pending/.
	existing := existingMemoryKeys(s)
	for _, c := range PendingCandidates(s) {
		existing = append(existing, normalizeKey(c.Description))
	}
	seen := map[string]bool{}
	written := 0
	for _, m := range slice {
		if m.Role != provider.RoleUser && m.Role != provider.RoleAssistant {
			continue
		}
		if isTransientBlock(m.Content) {
			continue
		}
		statement, reason := extractMemoryStatement(m.Content)
		if statement == "" {
			continue
		}
		key := normalizeKey(statement)
		if key == "" || seen[key] || existingCovers(existing, key) {
			continue
		}
		seen[key] = true
		name := candidateName(statement)
		cand := Candidate{
			Name:        name,
			Title:       candidateTitle(statement),
			Description: oneLine(statement),
			Type:        inferMemoryType(statement),
			Body:        statement + "\n\n**Why:** " + reason + "\n**How to apply:** verify against current project state before relying on it.",
			Reason:      reason,
			Evidence:    truncateRunes(oneLine(m.Content), 160),
		}
		b, err := json.Marshal(cand)
		if err != nil {
			return written, err
		}
		if err := os.WriteFile(pendingPath(s, name), b, 0o644); err != nil {
			return written, err
		}
		written++
	}
	if err := writeExtractCursor(s, ExtractCursor{SessionID: sessionID, ProcessedOffset: len(msgs)}); err != nil {
		return written, err
	}
	return written, nil
}

// PendingCandidates returns the staged candidates awaiting confirmation,
// sorted by name. A missing pending dir yields an empty list.
func PendingCandidates(s Store) []Candidate {
	if s.Dir == "" {
		return nil
	}
	dir := filepath.Join(s.Dir, pendingDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Candidate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var c Candidate
		if err := json.Unmarshal(b, &c); err != nil || c.Description == "" {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AcceptCandidate confirms a staged candidate: it is written into active
// memory and removed from pending. Returns the written memory path.
func AcceptCandidate(s Store, name string) (string, error) {
	if s.Dir == "" {
		return "", fmt.Errorf("memory store unavailable")
	}
	path := pendingPath(s, name)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("pending candidate %q: %w", name, err)
	}
	var c Candidate
	if err := json.Unmarshal(b, &c); err != nil {
		return "", fmt.Errorf("parse pending candidate %q: %w", name, err)
	}
	written, err := s.Save(Memory{
		Name:        c.Name,
		Title:       c.Title,
		Description: c.Description,
		Type:        NormalizeType(string(c.Type)),
		Body:        c.Body,
	})
	if err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return written, err
	}
	return written, nil
}

// RejectCandidate discards a staged candidate without touching active memory.
func RejectCandidate(s Store, name string) error {
	if s.Dir == "" {
		return fmt.Errorf("memory store unavailable")
	}
	err := os.Remove(pendingPath(s, name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// normalizeKey collapses whitespace for dedup comparison.
func normalizeKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// existingCovers reports whether any existing memory text overlaps the key
// (either direction), so a near-duplicate candidate is not staged.
func existingCovers(existing []string, key string) bool {
	if key == "" {
		return true
	}
	for _, text := range existing {
		if text == "" {
			continue
		}
		if strings.Contains(text, key) || strings.Contains(key, text) {
			return true
		}
		if sharesCorePhrase(text, key) {
			return true
		}
	}
	return false
}

// minSharedSubstrRunes is the smallest common substring that marks two
// statements as near-duplicates. Rules restated with different wording (e.g.
// "go build + 受影响包测试" vs "Go 代码改动 = go build + 受影响包测试") share their
// core phrase even though neither string fully contains the other.
const minSharedSubstrRunes = 10

// sharesCorePhrase reports whether two normalized statements share a common
// substring of at least minSharedSubstrRunes runes. It complements the
// contains-based check for reworded duplicates that no longer nest.
func sharesCorePhrase(a, b string) bool {
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) < minSharedSubstrRunes || len(rb) < minSharedSubstrRunes {
		return false
	}
	// Slide the shorter window over the longer text.
	short, long := ra, rb
	if len(short) > len(long) {
		short, long = long, short
	}
	for i := 0; i+minSharedSubstrRunes <= len(short); i++ {
		if strings.Contains(string(long), string(short[i:i+minSharedSubstrRunes])) {
			return true
		}
	}
	return false
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "..."
}

func hasAny(hay string, needles ...string) bool {
	hay = strings.ToLower(hay)
	for _, needle := range needles {
		if strings.Contains(hay, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
