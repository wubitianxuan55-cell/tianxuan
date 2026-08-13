package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"tianxuan/internal/frontmatter"
)

// Store is the per-project auto-memory: a directory of one-fact-per-file
// Markdown notes with frontmatter, plus a MEMORY.md index of one line per fact.
// The model maintains it through the `remember` tool; the index loads into the
// cached system-prompt prefix at boot so the model always knows what it has
// saved, and reads individual facts on demand with read_file. The whole thing is
// plain files the user can edit by hand.
type Store struct {
	Dir       string // ...tianxuan/projects/<slug>/memory (project-scoped)
	GlobalDir string // global memory directory (optional, for cross-project facts)
}

// Type classifies a memory, mirroring the auto-memory taxonomy.
type Type string

const (
	TypeUser      Type = "user"      // who the user is: role, preferences, expertise
	TypeFeedback  Type = "feedback"  // guidance on how to work (with why + how-to-apply)
	TypeProject   Type = "project"   // ongoing work / goals / constraints not in the code
	TypeReference Type = "reference" // pointers to external resources (URLs, tickets)
)

// Kind classifies a memory by cognitive function (LangMem-inspired).
// Orthogonal to Type — Kind controls how the memory is injected and retrieved.
type Kind string

const (
	KindSemantic   Kind = "semantic"   // facts, preferences, constraints → L1 prefix + search
	KindEpisodic   Kind = "episodic"   // past experiences, solutions → tag-triggered injection
	KindProcedural Kind = "procedural" // rules, best practices → always-on injection
)

// validTypes is the closed set the `remember` tool accepts; anything else
// normalises to TypeProject.
var validTypes = map[Type]bool{TypeUser: true, TypeFeedback: true, TypeProject: true, TypeReference: true}

// validKinds is the closed set for the Kind field.
var validKinds = map[Kind]bool{KindSemantic: true, KindEpisodic: true, KindProcedural: true}

// NormalizeType coerces an arbitrary string to a known Type, defaulting to
// TypeProject so a sloppy tool argument never blocks a save.
func NormalizeType(s string) Type {
	t := Type(strings.ToLower(strings.TrimSpace(s)))
	if validTypes[t] {
		return t
	}
	return TypeProject
}

// NormalizeKind coerces an arbitrary string to a known Kind, defaulting to
// KindSemantic so existing memories without an explicit kind stay semantic.
func NormalizeKind(s string) Kind {
	k := Kind(strings.ToLower(strings.TrimSpace(s)))
	if validKinds[k] {
		return k
	}
	return KindSemantic
}

// Memory is one stored fact.
type Memory struct {
	Name        string   // kebab-case slug; also the file stem (<name>.md)
	Title       string   // human-readable index label; falls back to a de-kebabed Name
	Description string   // one-line summary used for the index and recall
	Type        Type     // category: user / feedback / project / reference
	Kind        Kind     // cognitive function: semantic / episodic / procedural
	Tags        []string // trigger tags for episodic memories (empty for others)
	Body        string   // the fact itself (Markdown)
}

// ArchivedMemory is a saved fact that has been removed from active memory but
// kept on disk for traceability. The ArchivedAt timestamp records when it was
// archived; Path is the absolute path to the archived .md file.
type ArchivedMemory struct {
	Memory
	Path       string    `json:"path"`
	ArchivedAt time.Time `json:"archivedAt"`
}

// StoreFor resolves the auto-memory directory for a project working dir under
// the user config root, e.g. ~/.config/tianxuan/projects/-Users-me-proj/memory.
// A "" userDir (config dir unresolvable) yields a zero Store, which all methods
// treat as a disabled no-op.
func StoreFor(userDir, cwd string) Store {
	if userDir == "" {
		return Store{}
	}
	// Key the store by the nearest git root so every subdirectory of one
	// repository shares the same memory (Qwen/Claude convention); an
	// un-versioned working directory falls back to its own cwd slug.
	key := absOf(cwd)
	if root := gitRoot(absOf(cwd)); root != "" {
		key = root
	}
	return Store{
		Dir:       filepath.Join(userDir, "projects", slugify(key), "memory"),
		GlobalDir: filepath.Join(userDir, "memories"),
	}
}

// indexFile is the human-readable index of saved memories.
const indexFile = "MEMORY.md"

// slugify turns an absolute project path into a single filesystem-safe segment,
// matching the auto-memory convention (path separators → '-'), e.g.
// "/Users/me/proj" → "-Users-me-proj".
func slugify(absPath string) string {
	r := strings.NewReplacer(string(os.PathSeparator), "-", "/", "-", "\\", "-", ":", "-")
	return r.Replace(absPath)
}

// migrateLegacyStore moves a store created under a pre-git-root key (the cwd
// slug) into the git-root-keyed location so memory saved before that change is
// not orphaned when a session starts from a subdirectory. No-op when there is
// no git root, the legacy directory is missing, or the target already exists —
// migration never merges two stores.
func migrateLegacyStore(userDir, cwd string) error {
	if userDir == "" {
		return nil
	}
	abs := absOf(cwd)
	root := gitRoot(abs)
	if root == "" {
		return nil
	}
	oldKey := slugify(abs)
	newKey := slugify(root)
	if oldKey == newKey {
		return nil
	}
	oldDir := filepath.Join(userDir, "projects", oldKey, "memory")
	newDir := filepath.Join(userDir, "projects", newKey, "memory")
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return nil
	}
	if _, err := os.Stat(newDir); err == nil {
		return nil // target exists — never merge
	}
	if err := os.MkdirAll(filepath.Dir(newDir), 0o755); err != nil {
		return err
	}
	return os.Rename(oldDir, newDir)
}

// Index returns the merged MEMORY.md contents (the per-line index of saved
// memories) from the cross-project directory followed by the project one, or ""
// if there are none yet. This is what loads into the cached prefix.
func (s Store) Index() string {
	if s.Dir == "" && s.GlobalDir == "" {
		return ""
	}
	var parts []string
	for _, dir := range []string{s.GlobalDir, s.Dir} {
		if dir == "" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, indexFile))
		if err != nil {
			continue
		}
		if t := strings.TrimSpace(string(b)); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}

// Path returns the absolute file path a memory with the given name lives at.
func (s Store) Path(name string) string {
	return filepath.Join(s.Dir, slug(name)+".md")
}

// saveDir returns the directory a memory should be written to: user-type and
// feedback-type facts go cross-project (GlobalDir) when one is configured so
// they survive project switches; everything else stays project-scoped.
func (s Store) saveDir(m Memory) string {
	if s.GlobalDir != "" && s.GlobalDir != s.Dir {
		t := NormalizeType(string(m.Type))
		if t == TypeUser || t == TypeFeedback {
			return s.GlobalDir
		}
	}
	return s.Dir
}

// Save writes (or overwrites) a memory file and refreshes its MEMORY.md index
// line. It is the single mutation entry point — the `remember` tool, the desktop
// editor, and any future importer all go through here so the index never drifts
// from the files. Returns the path written.
func (s Store) Save(m Memory) (string, error) {
	if s.Dir == "" && s.GlobalDir == "" {
		return "", fmt.Errorf("memory store unavailable (no user config dir)")
	}
	name := slug(m.Name)
	if name == "" {
		return "", fmt.Errorf("memory needs a name")
	}
	dir := s.saveDir(m)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(render(m, name)), 0o644); err != nil {
		return "", err
	}
	if err := s.reindexIn(dir, name, m); err != nil {
		return path, err
	}
	return path, nil
}

// Archive moves a memory file to .archive/ instead of permanently deleting it,
// so wrong memories remain traceable and recoverable. The MEMORY.md index line
// is still removed. A missing file is not an error.
func (s Store) Archive(name string) (string, error) {
	if s.Dir == "" && s.GlobalDir == "" {
		return "", fmt.Errorf("memory store unavailable (no user config dir)")
	}
	name = slug(name)
	if name == "" {
		return "", fmt.Errorf("memory needs a name")
	}
	dir := s.findDir(name)
	if dir == "" {
		return "", nil // nothing to archive
	}
	file := name + ".md"
	src := filepath.Join(dir, file)
	archiveDir := filepath.Join(dir, ".archive")
	if err := os.MkdirAll(archiveDir, 0700); err != nil {
		return "", err
	}
	ts := time.Now().UTC().Format("20060102-150405.000")
	dest := filepath.Join(archiveDir, ts+"-"+file)
	if err := os.Rename(src, dest); err != nil {
		return "", err
	}
	if err := s.flushIndexIn(dir, s.indexLinesExceptIn(dir, name)); err != nil {
		return dest, err
	}
	return dest, nil
}

// Delete removes a memory — it archives first, then removes the index line.
// Uses Archive internally so wrong memories remain traceable in .archive/.
func (s Store) Delete(name string) error {
	_, err := s.Archive(name)
	return err
}

// ChangeType changes the Type of a saved memory (e.g. promote to "user" level
// or demote to "project"/"feedback"). The memory is reloaded from disk, its
// Type updated, and re-saved — all other fields are preserved.
func (s Store) ChangeType(name string, newType Type) error {
	if s.Dir == "" && s.GlobalDir == "" {
		return fmt.Errorf("memory store unavailable (no user config dir)")
	}
	name = slug(name)
	if name == "" {
		return fmt.Errorf("memory needs a name")
	}
	oldDir := s.findDir(name)
	if oldDir == "" {
		return fmt.Errorf("memory %q not found", name)
	}
	var target *Memory
	for _, m := range s.List() {
		if m.Name == name {
			copy := m
			target = &copy
			break
		}
	}
	if target == nil {
		return fmt.Errorf("memory %q not found", name)
	}
	target.Type = newType
	newDir := s.saveDir(*target)
	if _, err := s.Save(*target); err != nil {
		return err
	}
	if newDir != oldDir {
		// Move semantics: drop the file and index line from the old location.
		if err := os.Remove(filepath.Join(oldDir, name+".md")); err != nil {
			return err
		}
		return s.flushIndexIn(oldDir, s.indexLinesExceptIn(oldDir, name))
	}
	return nil
}

// render serializes a memory to frontmatter + body. The frontmatter mirrors the
// auto-memory shape (name / description / metadata.type / metadata.kind) so the
// files are interchangeable with that ecosystem and re-readable by loadMemory.
func render(m Memory, name string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + name + "\n")
	if t := oneLine(m.Title); t != "" {
		b.WriteString("title: " + t + "\n")
	}
	b.WriteString("description: " + oneLine(m.Description) + "\n")
	b.WriteString("metadata:\n")
	b.WriteString("  type: " + string(NormalizeType(string(m.Type))) + "\n")
	if k := NormalizeKind(string(m.Kind)); k != KindSemantic {
		b.WriteString("  kind: " + string(k) + "\n")
	}
	if len(m.Tags) > 0 {
		b.WriteString("  tags: [" + strings.Join(m.Tags, ", ") + "]\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(m.Body))
	b.WriteString("\n")
	return b.String()
}

// indexLineRe matches a managed index line so reindex/Delete can target the line
// for one memory by its filename without disturbing the rest of a hand-edited
// MEMORY.md.
var indexLineRe = regexp.MustCompile(`\]\(([^)]+)\.md\)`)

// findDir returns the store directory currently holding the memory file with
// the given slug name, or "" when it is not saved in either directory.
func (s Store) findDir(name string) string {
	for _, dir := range s.dirs() {
		if _, err := os.Stat(filepath.Join(dir, name+".md")); err == nil {
			return dir
		}
	}
	return ""
}

// indexLinesExceptIn returns the managed MEMORY.md lines of dir keyed by
// filename stem, dropping the entry for name (a missing index → empty map).
func (s Store) indexLinesExceptIn(dir, name string) map[string]string {
	existing, _ := os.ReadFile(filepath.Join(dir, indexFile))
	keep := map[string]string{}
	for _, line := range strings.Split(string(existing), "\n") {
		if mt := indexLineRe.FindStringSubmatch(line); mt != nil && mt[1] != name {
			keep[mt[1]] = strings.TrimRight(line, "\r")
		}
	}
	return keep
}

// flushIndexIn rewrites dir/MEMORY.md from the managed lines, sorted by filename.
func (s Store) flushIndexIn(dir string, lines map[string]string) error {
	names := make([]string, 0, len(lines))
	for n := range lines {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("# Memory\n\n")
	for _, n := range names {
		b.WriteString(lines[n])
		b.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(dir, indexFile), []byte(b.String()), 0o644)
}

// reindexIn rewrites the MEMORY.md line for name in dir, preserving every other
// managed line. The line is "- [<title>](<name>.md) — <description>"; title
// falls back to a de-kebabed name so the index reads as a label, never a bare slug.
// Kind is shown as a prefix tag when non-semantic: [E] for episodic, [P] for procedural.
func (s Store) reindexIn(dir, name string, m Memory) error {
	lines := s.indexLinesExceptIn(dir, name)
	kindTag := ""
	switch NormalizeKind(string(m.Kind)) {
	case KindEpisodic:
		kindTag = "[E] "
	case KindProcedural:
		kindTag = "[P] "
	}
	lines[name] = fmt.Sprintf("- %s[%s](%s.md) — %s", kindTag, displayTitle(m.Title, name), name, oneLine(m.Description))
	return s.flushIndexIn(dir, lines)
}

// List returns the saved memories parsed from their files, sorted by name. Used
// by `/memory` and the desktop memory panel. Files that fail to parse are
// skipped so one bad file never hides the rest.
func (s Store) List() []Memory {
	if s.Dir == "" && s.GlobalDir == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []Memory
	for _, dir := range s.dirs() {
		for _, m := range s.listIn(dir) {
			if seen[m.Name] {
				continue // project-scoped copy wins over the global one
			}
			seen[m.Name] = true
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListIn returns the saved memories of a single store directory (project or
// global), sorted by name. Used by the desktop panel to label each fact's
// scope (P1 cross-project design). A missing directory yields nil.
func (s Store) ListIn(dir string) []Memory {
	if dir == "" {
		return nil
	}
	out := s.listIn(dir)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// listIn reads the memory files of one directory without deduplication.
func (s Store) listIn(dir string) []Memory {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Memory
	for _, e := range entries {
		if e.IsDir() || e.Name() == indexFile || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if m, ok := loadMemory(filepath.Join(dir, e.Name())); ok {
			out = append(out, m)
		}
	}
	return out
}

// dirs returns the non-empty store directories to scan. Project-scoped Dir
// takes priority; GlobalDir (when set) is also included for cross-project facts.
func (s Store) dirs() []string {
	var out []string
	if s.Dir != "" {
		out = append(out, s.Dir)
	}
	if s.GlobalDir != "" && s.GlobalDir != s.Dir {
		out = append(out, s.GlobalDir)
	}
	return out
}

// ListArchived returns archived memories parsed from .archive/, newest first.
// Archived files stay out of List() and the prompt index, so stale facts remain
// inspectable without being reused as active truth.
func (s Store) ListArchived() []ArchivedMemory {
	if s.Dir == "" && s.GlobalDir == "" {
		return nil
	}
	var out []ArchivedMemory
	for _, base := range s.dirs() {
		if base == "" {
			continue
		}
		dir := filepath.Join(base, ".archive")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			m, ok := loadMemory(path)
			if !ok {
				continue
			}
			when := archiveTimeFromName(e.Name())
			if when.IsZero() {
				if info, err := e.Info(); err == nil {
					when = info.ModTime()
				}
			}
			out = append(out, ArchivedMemory{Memory: m, Path: path, ArchivedAt: when})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ArchivedAt.Equal(out[j].ArchivedAt) {
			return out[i].ArchivedAt.After(out[j].ArchivedAt)
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// archiveTimeFromName extracts the timestamp from an archived filename, which is
// prefixed with "20060102-150405.000-" by Archive.
func archiveTimeFromName(name string) time.Time {
	const stampLen = len("20060102-150405.000")
	if len(name) <= stampLen || name[stampLen] != '-' {
		return time.Time{}
	}
	t, err := time.Parse("20060102-150405.000", name[:stampLen])
	if err != nil {
		return time.Time{}
	}
	return t
}

// loadMemory parses one fact file back into a Memory. It tolerates the minimal
// frontmatter render writes; a file without frontmatter still loads with its
// body and a name derived from the filename.
func loadMemory(path string) (Memory, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Memory{}, false
	}
	fm, body := splitFrontmatter(string(b))
	m := Memory{
		Name:        fm["name"],
		Title:       fm["title"],
		Description: fm["description"],
		Type:        NormalizeType(fm["type"]),
		Kind:        NormalizeKind(fm["kind"]),
		Tags:        parseTags(fm["tags"]),
		Body:        strings.TrimSpace(body),
	}
	if m.Name == "" {
		m.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	return m, true
}

// splitFrontmatter is a thin wrapper; the real parser lives in
// internal/frontmatter.
func splitFrontmatter(s string) (map[string]string, string) {
	return frontmatter.Split(s)
}

// slugRe strips everything but lowercase alphanumerics and dashes.
var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slug normalises a name into a kebab-case, filesystem-safe stem.
func slug(s string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-"), "-")
}

// oneLine collapses whitespace so a description can't break the single-line
// index or frontmatter format.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// displayTitle is the index link label: the given title, or a de-kebabed name
// when none was supplied, so a bare slug never leaks into the index.
func displayTitle(title, name string) string {
	if t := oneLine(title); t != "" {
		return t
	}
	return strings.ReplaceAll(name, "-", " ")
}

// parseTags parses a frontmatter tags value. Accepts JSON array syntax
// [a, b, c] or comma-separated plain text. Returns nil for empty input.
func parseTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	// JSON array: [a, b, c] or ["a", "b"]
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		inner := strings.TrimSpace(raw[1 : len(raw)-1])
		if inner == "" {
			return nil
		}
		parts := strings.Split(inner, ",")
		var out []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			p = strings.Trim(p, "\"'")
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	// Plain comma-separated
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
