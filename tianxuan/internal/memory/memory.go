package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Set is everything memory loaded for one session: the hierarchical docs and a
// handle to the auto-memory store (whose index is captured at load time). It is
// assembled once at boot and folded into the system prompt by Compose. CWD and
// UserDir are retained so the controller can resolve quick-add targets without
// re-deriving discovery context.
type Set struct {
	Docs    []Source // TIANXUAN.md / AGENTS.md / REASONIX.md, ascending precedence
	Store   Store    // auto-memory store (may be a zero/disabled Store)
	Index   string   // MEMORY.md contents at load time
	CWD     string   // project working dir used for discovery
	UserDir string   // user config root (may be "")
}

// Options configures discovery. CWD defaults to "." and UserDir is the user
// config root (config.MemoryUserDir()); a "" UserDir disables user-global docs
// and the auto-memory store.
type Options struct {
	CWD     string
	UserDir string
}

// Load discovers all memory for a session: the hierarchical docs and the
// auto-memory index. It is best-effort and never errors — missing files just
// mean less memory — so boot can call it unconditionally.
func Load(opts Options) *Set {
	cwd := opts.CWD
	if cwd == "" {
		cwd = "."
	}
	store := StoreFor(opts.UserDir, cwd)
	return &Set{
		Docs:    discoverDocs(cwd, opts.UserDir),
		Store:   store,
		Index:   store.Index(),
		CWD:     cwd,
		UserDir: opts.UserDir,
	}
}

// RefreshDocs re-discovers only the hierarchical doc memory (TIANXUAN.md chain)
// without touching the auto-memory index. Used after QuickAdd / SaveDoc so the
// full Load() cost (which also reads all .md fact files) is avoided.
func (s *Set) RefreshDocs() {
	s.Docs = discoverDocs(s.CWD, s.UserDir)
}

// RefreshIndex re-reads only the MEMORY.md index without touching doc discovery
// or individual memory files. Used after remember/forget so the Index stays in
// sync with the active memory set.
func (s *Set) RefreshIndex() {
	s.Index = s.Store.Index()
}

// DocPath returns the doc-memory file a given scope writes to. To avoid splitting
// a project's memory across conventions, it prefers a file that already exists
// (TIANXUAN.md / AGENTS.md / REASONIX.md / CLAUDE.md, in that order); when none
// exists it creates the universal default (AGENTS.md / AGENTS.local.md).
// ScopeUser → <userDir>, ScopeLocal → <cwd> with the *.local.md names, anything
// else → <cwd>. Returns "" for ScopeUser when no user dir is configured.
func (s *Set) DocPath(scope Scope) string {
	dir := s.CWD
	names, def := docNames, defaultDocName
	switch scope {
	case ScopeUser:
		if s.UserDir == "" {
			return ""
		}
		dir = s.UserDir
	case ScopeLocal:
		names, def = localNames, defaultLocalName
	}
	for _, n := range names {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err == nil {
			return p // append to the doc already in use
		}
	}
	return filepath.Join(dir, def)
}

// Empty reports whether the set carries nothing to inject, so Compose can leave
// the base prompt byte-for-byte untouched (and the cache prefix maximal) when
// there is no memory at all.
func (s *Set) Empty() bool {
	return s == nil || (len(s.Docs) == 0 && strings.TrimSpace(s.Index) == "")
}

// docScopes are the scopes the panel can target for a quick-add or a new doc.
// Ordered broad → specific for display.
var docScopes = []Scope{ScopeUser, ScopeProject, ScopeLocal}

// allowedDocPaths is the closed set of files WriteDoc / AppendDoc may touch: the
// canonical file for each writable scope, plus every doc already discovered this
// session (so an ancestor or AGENTS.md the user is already editing stays
// editable). Keyed by absolute path. This bounds frontend-driven writes to real
// memory files rather than arbitrary paths.
func (s *Set) allowedDocPaths() map[string]bool {
	allow := map[string]bool{}
	for _, sc := range docScopes {
		if p := s.DocPath(sc); p != "" {
			allow[absOf(p)] = true
		}
	}
	for _, d := range s.Docs {
		allow[absOf(d.Path)] = true
	}
	return allow
}

// WriteDoc overwrites a doc-memory file with body, after checking path is a
// recognized memory file (see allowedDocPaths). It is the save side of the
// desktop panel's in-place editor. The write lands on disk immediately but does
// NOT mutate the cache-stable system prefix — the edit folds into the prefix on
// the next session; to make it apply this session, the controller separately
// queues a turn-tail note. Returns the path written.
func (s *Set) WriteDoc(path, body string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("memory unavailable")
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("no path given")
	}
	if !s.allowedDocPaths()[absOf(path)] {
		return "", fmt.Errorf("refusing to write %q: not a recognized memory file", path)
	}
	return path, writeDocFile(path, body)
}

// Block renders the memory as a single Markdown section, or "" when empty. It is
// deterministic given the same files, which is what keeps it a stable cache
// prefix across sessions that don't change their memory.
//
// Doc bodies load in full; the MEMORY.md index is included as a one-line-per-fact
// summary. The model reads individual saved facts on demand via the `memory` tool
// rather than keeping all bodies in the prefix.
func (s *Set) Block() string {
	if s.Empty() {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Memory\n\n")

	for _, d := range s.Docs {
		fmt.Fprintf(&b, "\n## %s (%s)\n\n%s\n", d.Path, d.Scope, strings.TrimSpace(d.Body))
	}

	if idx := strings.TrimSpace(s.Index); idx != "" {
		b.WriteString("\n## Saved memories\n\n")
		b.WriteString("Facts you saved in earlier sessions — use read_file to see details.\n\n")
		b.WriteString(idx)
		var dirs []string
		for _, d := range s.Store.dirs() {
			if d != "" {
				dirs = append(dirs, d)
			}
		}
		fmt.Fprintf(&b, "\n\n(stored under %s)\n", strings.Join(dirs, " and "))
		// Aging: count stale memories (>90 days since last update).
		if stale := countStale(s.Store.List(), 90); stale > 0 {
			fmt.Fprintf(&b, "(%d memories not updated in >90 days — use `memory list` to review)\n", stale)
		}
	}
	return b.String()
}

func countStale(memories []Memory, days int) int {
	cutoff := time.Now().AddDate(0, 0, -days)
	n := 0
	for _, m := range memories {
		if !m.Mtime.IsZero() && m.Mtime.Before(cutoff) {
			n++
		}
	}
	return n
}

// Compose folds the memory block onto the base system prompt and returns the
// durable cached-prefix string. Base stays first (it is the most stable text, so
// it remains a valid cache prefix even when memory changes between sessions);
// memory follows. With no memory, base is returned unchanged.
func Compose(base string, s *Set) string {
	block := s.Block()
	if block == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return block
	}
	return strings.TrimRight(base, "\n") + "\n\n" + block
}

// SearchMatch is a single search result with a relevance score.
type SearchMatch struct {
	Name  string // memory slug
	Score int    // number of matching tokens (higher = more relevant)
}

// Search finds memories matching the query using simple token matching.
// Returns nil when the store is empty or no matches found.
// Used by controller for /memories command and dream distillation.
func (s *Set) Search(query string) []SearchMatch {
	if s == nil {
		return nil
	}
	memories := s.Store.List()
	if len(memories) == 0 {
		return nil
	}

	query = strings.ToLower(strings.TrimSpace(query))
	queryTokens := strings.FieldsFunc(query, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
	})
	if len(queryTokens) == 0 {
		return nil
	}

	type scored struct {
		name  string
		score int
	}
	var results []scored
	for _, m := range memories {
		text := strings.ToLower(m.Title + " " + m.Description + " " + m.Body)
		score := 0
		for _, token := range queryTokens {
			if len(token) >= 2 && strings.Contains(text, token) {
				score++
			}
		}
		if score > 0 {
			results = append(results, scored{m.Name, score})
		}
	}
	if len(results) == 0 {
		return nil
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].name < results[j].name
	})

	matches := make([]SearchMatch, len(results))
	for i, r := range results {
		matches[i] = SearchMatch{Name: r.name, Score: r.score}
	}
	return matches
}
