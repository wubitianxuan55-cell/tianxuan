package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Set is everything memory loaded for one session: the hierarchical docs and a
// handle to the auto-memory store (whose index is captured at load time). It is
// assembled once at boot and folded into the system prompt by Compose. CWD and
// UserDir are retained so the controller can resolve quick-add targets without
// re-deriving discovery context.
type Set struct {
	Docs    []Source      // TIANXUAN.md / AGENTS.md, ascending precedence
	Store   Store         // auto-memory store (may be a zero/disabled Store)
	Index   string        // MEMORY.md contents at load time
	Search  *SearchIndex  // V5.31: in-memory inverted index for memory_search
	CWD     string        // project working dir used for discovery
	UserDir string        // user config root (may be "")
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
		Search:  store.BuildSearchIndex(),
		CWD:     cwd,
		UserDir: opts.UserDir,
	}
}

// DocPath returns the doc-memory file a given scope writes to. To avoid splitting
// a project's memory across conventions, it prefers a file that already exists
// (TIANXUAN.md / AGENTS.md / CLAUDE.md, in that order); when none exists it
// creates the universal default (AGENTS.md / AGENTS.local.md). ScopeUser →
// <userDir>, ScopeLocal → <cwd> with the *.local.md names, anything else → <cwd>.
// Returns "" for ScopeUser when no user dir is configured.
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

// Block renders memory for the cache-stable prefix. Returns a compact block when
// the full memory would exceed a reasonable size, keeping the prefix lean.
//
// V5.30: doc bodies larger than 4 KiB total are replaced with their paths and
// first line only — the controller injects the full bodies at turn-tail via
// DocBlock, so the model still sees them without expanding the cache prefix.
func (s *Set) Block() string {
	if s.Empty() {
		return ""
	}
	full := s.buildFullBlock()
	if len(full) <= 4096 {
		return full // small memory → everything in prefix
	}
	return s.buildCompactBlock()
}

// DocBlock returns just the doc bodies for turn-tail injection (V5.30).
// The controller calls this in the first turn to give the model full doc content
// without expanding the cache-stable prefix.
func (s *Set) DocBlock() string {
	if len(s.Docs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Memory docs (loaded from turn-tail)\n\n")
	for _, d := range s.Docs {
		fmt.Fprintf(&b, "\n## %s (%s)\n\n%s\n", d.Path, d.Scope, strings.TrimSpace(d.Body))
	}
	return b.String()
}

// buildFullBlock returns the complete memory block (docs + index).
func (s *Set) buildFullBlock() string {
	var b strings.Builder
	b.WriteString("# Memory\n\n")

	for _, d := range s.Docs {
		fmt.Fprintf(&b, "\n## %s (%s)\n\n%s\n", d.Path, d.Scope, strings.TrimSpace(d.Body))
	}

	if idx := strings.TrimSpace(s.Index); idx != "" {
		b.WriteString("\n## Saved memories\n\n")
		b.WriteString("Facts you saved in earlier sessions — use read_file to see details.\n\n")
		b.WriteString(idx)
		fmt.Fprintf(&b, "\n\n(stored under %s)\n", s.Store.Dir)
	}
	return b.String()
}

// buildCompactBlock returns an abbreviated memory block for the cache prefix.
// Includes doc paths and first line, plus the complete MEMORY.md index.
func (s *Set) buildCompactBlock() string {
	var b strings.Builder
	b.WriteString("# Memory\n\nDocs available:\n\n")
	for _, d := range s.Docs {
		first := strings.TrimSpace(d.Body)
		if idx := strings.Index(first, "\n"); idx >= 0 {
			first = first[:idx]
		}
		first = strings.TrimSpace(first)
		if len(first) > 160 {
			first = first[:160] + "\u2026"
		}
		fmt.Fprintf(&b, "- %s (%s): %s\n", filepath.Base(d.Path), d.Scope, first)
	}

	if idx := strings.TrimSpace(s.Index); idx != "" {
		b.WriteString("\n## Saved memories\n\n")
		b.WriteString(idx)
		fmt.Fprintf(&b, "\n\n(stored under %s)\n", s.Store.Dir)
	}
	return b.String()
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

// InitDefaults creates default memory files when a project has none. It writes
// AGENTS.md with a minimal template so the memory panel isn't empty on first
// open. Existing files are never overwritten.
func InitDefaults(s *Set) {
	if s == nil {
		return
	}
	path := s.DocPath(ScopeProject)
	if _, err := os.Stat(path); err == nil {
		return // already exists
	}
	defaultContent := `# Project memory

## Notes

<!-- 在这里记录项目约定、架构决策、编码规范等。AI 助手每轮对话前会读取此文件。 -->

`
	if err := os.WriteFile(path, []byte(defaultContent), 0644); err != nil {
		return
	}
	// Reload so the new doc appears immediately
	s.Docs = discoverDocs(s.CWD, s.UserDir)
}
