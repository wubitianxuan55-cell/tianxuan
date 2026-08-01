package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// strengthFile persists per-memory access strength (agentmemory-style
// reinforcement: frequently accessed memories strengthen, cold ones decay).
const strengthFile = "strength.json"

// Strength records how often a memory was recalled and when it was last
// accessed, used for reinforcement and TTL eviction.
type Strength struct {
	Count        int       `json:"count"`
	LastAccessAt time.Time `json:"lastAccessAt"`
}

// readStrengthMap loads the full strength table (empty when absent).
func readStrengthMap(s Store) (map[string]Strength, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, strengthFile))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Strength{}, nil
		}
		return nil, err
	}
	var m map[string]Strength
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", strengthFile, err)
	}
	if m == nil {
		m = map[string]Strength{}
	}
	return m, nil
}

func writeStrengthMap(s Store, m map[string]Strength) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, strengthFile), b, 0o644)
}

// ReadStrength returns the recorded strength for one memory (zero value when
// it has never been reinforced).
func ReadStrength(s Store, name string) (Strength, error) {
	m, err := readStrengthMap(s)
	if err != nil {
		return Strength{}, err
	}
	return m[slug(name)], nil
}

// WriteStrength records strength for one memory, preserving every other entry.
func WriteStrength(s Store, name string, st Strength) error {
	m, err := readStrengthMap(s)
	if err != nil {
		return err
	}
	m[slug(name)] = st
	return writeStrengthMap(s, m)
}

// ReinforceAccess bumps the recall counter for one memory and refreshes its
// last-access time. Called whenever auto-recall injects the memory.
func ReinforceAccess(s Store, name string) error {
	m, err := readStrengthMap(s)
	if err != nil {
		return err
	}
	key := slug(name)
	cur := m[key]
	cur.Count++
	cur.LastAccessAt = time.Now()
	m[key] = cur
	return writeStrengthMap(s, m)
}

// EvictStale archives memories that have been cold past ttl and rarely
// recalled, mirroring agentmemory's TTL eviction (archive keeps them
// traceable and recoverable — never a hard delete). Returns the number
// archived. A memory without strength data is left alone.
func EvictStale(s Store, ttl time.Duration, now time.Time) (int, error) {
	if s.Dir == "" {
		return 0, nil
	}
	m, err := readStrengthMap(s)
	if err != nil {
		return 0, err
	}
	archived := 0
	for _, mem := range s.List() {
		st, ok := m[mem.Name]
		if !ok || st.LastAccessAt.IsZero() {
			continue
		}
		if now.Sub(st.LastAccessAt) <= ttl {
			continue
		}
		if st.Count > 2 {
			continue // strongly reinforced — survives decay
		}
		if _, err := s.Archive(mem.Name); err != nil {
			return archived, err
		}
		archived++
	}
	return archived, nil
}

// ProjectProfile is a deterministic summary of the saved memory set (agentmemory
// profile distillation): dominant concepts, per-type distribution, and common
// error topics. Built from disk, persisted under profile.json.
type ProjectProfile struct {
	UpdatedAt     time.Time    `json:"updatedAt"`
	TotalMemories int          `json:"totalMemories"`
	TopConcepts   []string     `json:"topConcepts"`
	TopTypes      map[Type]int `json:"topTypes"`
	CommonErrors  []string     `json:"commonErrors"`
}

// profileFile persists the computed project profile.
const profileFile = "profile.json"

// BuildProfile condenses the saved memories into a project profile and
// persists it. Deterministic — no LLM call.
func BuildProfile(s Store) (ProjectProfile, error) {
	if s.Dir == "" {
		return ProjectProfile{}, nil
	}
	mems := s.List()
	conceptFreq := map[string]int{}
	typeCounts := map[Type]int{}
	var commonErrors []string
	for _, m := range mems {
		typeCounts[NormalizeType(string(m.Type))]++
		text := strings.ToLower(m.Title + " " + m.Description + " " + m.Body)
		for _, tok := range profileTokens(text) {
			conceptFreq[tok]++
		}
		if isErrorTopic(m) && len(commonErrors) < 3 {
			commonErrors = append(commonErrors, m.Title)
		}
	}

	type freq struct {
		word  string
		count int
	}
	var ranked []freq
	for w, c := range conceptFreq {
		ranked = append(ranked, freq{w, c})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].word < ranked[j].word
	})
	top := make([]string, 0, 5)
	for i, f := range ranked {
		if i >= 5 {
			break
		}
		top = append(top, f.word)
	}

	p := ProjectProfile{
		UpdatedAt:     time.Now(),
		TotalMemories: len(mems),
		TopConcepts:   top,
		TopTypes:      typeCounts,
		CommonErrors:  commonErrors,
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return p, err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if err := os.WriteFile(filepath.Join(s.Dir, profileFile), b, 0o644); err != nil {
		return p, err
	}
	return p, nil
}

// ReadProfile loads the persisted project profile (zero value + no error when
// none has been built yet). Best-effort for the panel's overview card.
func ReadProfile(s Store) (ProjectProfile, error) {
	if s.Dir == "" {
		return ProjectProfile{}, nil
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, profileFile))
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectProfile{}, nil
		}
		return ProjectProfile{}, err
	}
	var p ProjectProfile
	if err := json.Unmarshal(b, &p); err != nil {
		return ProjectProfile{}, fmt.Errorf("parse %s: %w", profileFile, err)
	}
	return p, nil
}

// profileStopwords are common English words that add no concept signal.
var profileStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "this": true,
	"that": true, "from": true, "have": true, "are": true, "was": true,
	"use": true, "using": true, "when": true, "into": true, "project": true,
	"memory": true, "description": true, "body": true,
}

// profileTokens extracts lowercase alphanumeric word tokens, dropping
// stopwords and short fragments.
func profileTokens(text string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(text, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len(f) >= 2 && !profileStopwords[f] {
			out = append(out, f)
		}
	}
	return out
}

// isErrorTopic reports whether a memory reads as a known-problem record.
func isErrorTopic(m Memory) bool {
	text := strings.ToLower(m.Title + " " + m.Description + " " + m.Body)
	for _, kw := range []string{"error", "bug", "fail", "报错", "错误", "失败", "坑"} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}
