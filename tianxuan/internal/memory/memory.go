package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	docs := discoverDocs(cwd, opts.UserDir)
	return &Set{
		Docs:    docs,
		Store:   store,
		Index:   store.Index(),
		Search:  store.BuildSearchIndex(docs),
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

// PlannerBlock returns memory filtered for the Hermes planner: project
// context (repo, architecture, skills, cache rules, role descriptions) minus
// executor-only coding disciplines (TDD, verification, no-placeholders, plan
// granularity, Superpowers methodology). The planner is read-only — coding
// rules waste tokens and can confuse role attribution ("Hephaestus (you)"
// read by Hermes).
func (s *Set) PlannerBlock() string {
	if s.Empty() {
		return ""
	}
	full := s.buildPlannerFullBlock()
	if len(full) <= 4096 {
		return full
	}
	return s.buildPlannerCompactBlock()
}

// buildPlannerFullBlock is buildFullBlock with AGENTS.md-class docs filtered
// for planner relevance.
func (s *Set) buildPlannerFullBlock() string {
	var b strings.Builder
	b.WriteString("# Memory\n\n")
	if profile := s.ProfileBlock(); profile != "" {
		b.WriteString(profile + "\n\n")
	}
	for _, d := range s.Docs {
		body := d.Body
		if isAgentDoc(d.Path) {
			body = filterAGENTSForPlanner(body)
		}
		fmt.Fprintf(&b, "\n## %s (%s)\n\n%s\n", d.Path, d.Scope, strings.TrimSpace(body))
	}
	if idx := strings.TrimSpace(s.Index); idx != "" {
		b.WriteString("\n## Saved memories\n\n")
		b.WriteString(idx)
		fmt.Fprintf(&b, "\n\n(stored under %s)\n", s.Store.Dir)
	}
	return b.String()
}

// buildPlannerCompactBlock is buildCompactBlock with AGENTS.md-class docs
// filtered for planner relevance.
func (s *Set) buildPlannerCompactBlock() string {
	var b strings.Builder
	b.WriteString("# Memory\n\n")
	if profile := s.ProfileBlock(); profile != "" {
		b.WriteString(profile + "\n\n")
	}
	b.WriteString("Docs available:\n\n")
	for _, d := range s.Docs {
		first := strings.TrimSpace(d.Body)
		if isAgentDoc(d.Path) {
			first = filterAGENTSForPlanner(first)
		}
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

// isAgentDoc reports whether path is an AGENTS.md-class project memory file
// whose content mixes planner-relevant context with executor-only rules.
func isAgentDoc(path string) bool {
	base := filepath.Base(path)
	return base == "AGENTS.md" || base == "TIANXUAN.md" || base == "CLAUDE.md" ||
		base == "AGENTS.local.md" || base == "TIANXUAN.local.md" || base == "CLAUDE.local.md"
}

// filterAGENTSForPlanner removes executor-only sections from AGENTS.md-class
// content: encoding iron laws (TDD/verification/no-placeholders/…), plan
// granularity, and the Superpowers methodology appendix. It also fixes the
// "Hephaestus (执行者，你)" address so the planner (Hermes) isn't confused.
func filterAGENTSForPlanner(body string) string {
	// 1. Drop the Superpowers appendix (last major section in AGENTS.md).
	if idx := strings.Index(body, "\n## 🦸 Superpowers"); idx >= 0 {
		body = strings.TrimRight(body[:idx], "\n")
	}

	// 2. Strip encoding iron laws except "拒绝谄媚" — the planner interacts
	//    with the user (ask, plan confirmation, feedback) and must resist
	//    sycophancy. Other iron laws (TDD, verification, etc.) are executor-only.
	const ironHeader = "- **编码铁律**（自动生效）：\n"
	const ironEndMarker = "- **子代理隔离**："
	if start := strings.Index(body, ironHeader); start >= 0 {
		if end := strings.Index(body[start:], ironEndMarker); end >= 0 {
			ironBlock := body[start+len(ironHeader) : start+end]
			var kept string
			for _, line := range strings.SplitAfter(ironBlock, "\n") {
				if strings.Contains(line, "拒绝谄媚") {
					// Rewrite: "  - 🔴 **拒绝谄媚**：..." → "- **拒绝谄媚**：..."
					line = strings.TrimPrefix(line, "  - 🔴 **")
					kept = "- **" + line
					break
				}
			}
			body = body[:start] + kept + body[start+end:]
		}
	}

	// 3. Drop the plan-granularity line (a standalone list item between 工具直用优先 and 核心约束).
	const planLine = "- **计划粒度**："
	if start := strings.Index(body, planLine); start >= 0 {
		end := strings.Index(body[start:], "\n")
		if end >= 0 {
			body = body[:start] + body[start+end+1:]
		}
	}

	// 4. The "executor (you)" address confuses Hermes.
	body = strings.Replace(body, "（执行者，你）", "（执行者）", 1)

	return body
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

// buildFullBlock returns the complete memory block (docs + index + profile).
func (s *Set) buildFullBlock() string {
	var b strings.Builder
	b.WriteString("# Memory\n\n")

	// User profile: auto-aggregated from user-type semantic memories.
	if profile := s.ProfileBlock(); profile != "" {
		b.WriteString(profile + "\n\n")
	}

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
	b.WriteString("# Memory\n\n")

	// User profile: auto-aggregated from user-type semantic memories.
	if profile := s.ProfileBlock(); profile != "" {
		b.WriteString(profile + "\n\n")
	}

	b.WriteString("Docs available:\n\n")
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

// ─── LangMem-inspired kind-aware memory blocks ───────────────────────────

// ProfileBlock auto-aggregates Type=user semantic memories into a structured
// user profile. Only semantic memories are included (episodic and procedural are
// handled separately). Returns "" when there are no user-typed semantic memories.
func (s *Set) ProfileBlock() string {
	if s == nil {
		return ""
	}
	memories := s.Store.List()
	var userFacts []string
	for _, m := range memories {
		if m.Kind != KindSemantic || m.Type != TypeUser {
			continue
		}
		if d := strings.TrimSpace(m.Description); d != "" {
			userFacts = append(userFacts, "- "+d)
		}
	}
	if len(userFacts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## User Profile (auto-aggregated)\n")
	for _, f := range userFacts {
		b.WriteString(f + "\n")
	}
	return b.String()
}

// ProceduralBlock returns all procedural memories as an always-active rules block.
// These are injected every turn, not just at boot. Returns "" when there are none.
func (s *Set) ProceduralBlock() string {
	if s == nil {
		return ""
	}
	memories := s.Store.List()
	var rules []string
	for _, m := range memories {
		if m.Kind != KindProcedural {
			continue
		}
		body := strings.TrimSpace(m.Body)
		if body == "" {
			continue
		}
		rules = append(rules, body)
	}
	if len(rules) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<procedural-rules>\n")
	b.WriteString("These rules ALWAYS apply — follow them in every response:\n\n")
	for i, r := range rules {
		fmt.Fprintf(&b, "%d. %s\n", i+1, r)
	}
	b.WriteString("</procedural-rules>")
	return b.String()
}

// EpisodicMatches finds episodic memories whose tags match any tokens in the
// input text. Used to inject relevant past experiences as few-shot context.
// Returns at most 3 matches, sorted by tag overlap count.
func (s *Set) EpisodicMatches(input string) []Memory {
	if s == nil || input == "" {
		return nil
	}
	memories := s.Store.List()
	inputLower := strings.ToLower(input)
	type scored struct {
		m     Memory
		score int
	}
	var candidates []scored
	for _, m := range memories {
		if m.Kind != KindEpisodic || len(m.Tags) == 0 {
			continue
		}
		overlap := 0
		for _, tag := range m.Tags {
			if strings.Contains(inputLower, strings.ToLower(tag)) {
				overlap++
			}
		}
		if overlap > 0 {
			candidates = append(candidates, scored{m, overlap})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	out := make([]Memory, len(candidates))
	for i, c := range candidates {
		out[i] = c.m
	}
	return out
}

// EpisodicBlock formats episodic memories as few-shot examples for turn-tail
// injection. Uses the observation→action→result pattern where available.
func EpisodicBlock(mm []Memory) string {
	if len(mm) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<episodic-memory>\n")
	b.WriteString("Past experiences relevant to the current task:\n\n")
	for _, m := range mm {
		b.WriteString(fmt.Sprintf("## %s\n", m.Title))
		b.WriteString(strings.TrimSpace(m.Body) + "\n\n")
	}
	b.WriteString("Use these past experiences to inform your approach — avoid repeating mistakes, apply successful patterns.\n")
	b.WriteString("</episodic-memory>")
	return b.String()
}

// ─── LinkGraph: [[memory-name]] cross-reference resolver ──────────────────

// linkRefRe matches [[name]] references in memory bodies.
var linkRefRe = regexp.MustCompile(`\[\[([a-zA-Z0-9](?:[a-zA-Z0-9._-]*[a-zA-Z0-9])?)\]\]`)

// InitDefaults creates default memory files when a project or user config has
// none. It writes AGENTS.md at both the user-global level (shared across all
// projects) and the project level (project-specific). Existing files are never
// overwritten.
func InitDefaults(s *Set) {
	if s == nil {
		return
	}
	// 用户级记忆：所有项目共享
	if s.UserDir != "" {
		userPath := filepath.Join(s.UserDir, defaultDocName)
		if _, err := os.Stat(userPath); os.IsNotExist(err) {
			os.WriteFile(userPath, []byte(userDefaultContent), 0644)
		}
	}
	// 项目级记忆：当前项目专属
	projPath := s.DocPath(ScopeProject)
	if _, err := os.Stat(projPath); os.IsNotExist(err) {
		os.WriteFile(projPath, []byte(projectDefaultContent), 0644)
	}
	// Reload so the new docs appear immediately
	s.Docs = discoverDocs(s.CWD, s.UserDir)
}

const userDefaultContent = `# User memory

## Preferences

<!-- 在这里记录你的个人偏好、工作习惯、常用约定等。所有项目共享此文件。 -->

- 思考输出说中文
`

const projectDefaultContent = `# Project memory

## Notes

<!-- 在这里记录项目约定、架构决策、编码规范等。每个项目独立。 -->

`
