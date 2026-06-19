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
//
// Memories of type "user" and "feedback" are routed to GlobalDir (shared across
// all projects), while "project" and "reference" stay in the project-specific Dir.
// List() and Index() merge both directories so every session sees the full set.
type Store struct {
	Dir       string // ...tianxuan/projects/<slug>/memory
	GlobalDir string // ...tianxuan/memory/global (shared across projects)
}

// Type classifies a memory, mirroring the auto-memory taxonomy.
type Type string

const (
	TypeUser      Type = "user"      // who the user is: role, preferences, expertise
	TypeFeedback  Type = "feedback"  // guidance on how to work (with why + how-to-apply)
	TypeProject   Type = "project"   // ongoing work / goals / constraints not in the code
	TypeReference Type = "reference" // pointers to external resources (URLs, tickets)
)

// validTypes is the closed set the `remember` tool accepts; anything else
// normalises to TypeProject.
var validTypes = map[Type]bool{TypeUser: true, TypeFeedback: true, TypeProject: true, TypeReference: true}

// NormalizeType coerces an arbitrary string to a known Type, defaulting to
// TypeProject so a sloppy tool argument never blocks a save.
func NormalizeType(s string) Type {
	t := Type(strings.ToLower(strings.TrimSpace(s)))
	if validTypes[t] {
		return t
	}
	return TypeProject
}

// Memory is one stored fact.
type Memory struct {
	Name        string    // kebab-case slug; also the file stem (<name>.md)
	Title       string    // human-readable index label; falls back to a de-kebabed Name
	Description string    // one-line summary used for the index and recall
	Type        Type
	Body        string    // the fact itself (Markdown)
	Mtime       time.Time // last modification time from the filesystem
}

// ArchivedMemory is a saved fact that has been removed from active memory but
// kept on disk for traceability.
type ArchivedMemory struct {
	Memory
	Path       string
	ArchivedAt time.Time
}

// StoreFor resolves the auto-memory directory for a project working dir under
// the user config root, e.g. ~/.config/tianxuan/projects/-Users-me-proj/memory.
// A "" userDir (config dir unresolvable) yields a zero Store, which all methods
// treat as a disabled no-op. GlobalDir is set to <userDir>/memory/global so
// user/feedback memories are shared across all projects.
func StoreFor(userDir, cwd string) Store {
	if userDir == "" {
		return Store{}
	}
	return Store{
		Dir:       filepath.Join(userDir, "projects", slugify(absOf(cwd)), "memory"),
		GlobalDir: filepath.Join(userDir, "memory", "global"),
	}
}

// dirs returns the writable directories: GlobalDir first (higher precedence for
// user/feedback types), then Dir. Empty strings are skipped; both may be ""
// (disabled store).
func (s Store) dirs() []string {
	var out []string
	if s.GlobalDir != "" {
		out = append(out, s.GlobalDir)
	}
	if s.Dir != "" {
		out = append(out, s.Dir)
	}
	return out
}

// isGlobalType reports whether a memory type should be stored globally.
func isGlobalType(t Type) bool {
	return t == TypeUser || t == TypeFeedback
}

// saveDir picks the target directory for a memory type: GlobalDir for
// user/feedback, Dir for project/reference.
func (s Store) saveDir(t Type) string {
	if isGlobalType(t) && s.GlobalDir != "" {
		return s.GlobalDir
	}
	return s.Dir
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

// Index returns the merged MEMORY.md contents from both Dir and GlobalDir,
// or "" if there are none yet. This is what loads into the cached prefix.
func (s Store) Index() string {
	var parts []string
	for _, dir := range s.dirs() {
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
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// Path returns the absolute file path a memory with the given name lives at.
// For global types (user/feedback), prefers GlobalDir; falls back to Dir.
func (s Store) Path(name string) string {
	name = slug(name)
	// Check global first for global types — if the file exists there, use it.
	if s.GlobalDir != "" {
		p := filepath.Join(s.GlobalDir, name+".md")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(s.Dir, name+".md")
}

// Save writes (or overwrites) a memory file and refreshes its MEMORY.md index
// line. It is the single mutation entry point — the `remember` tool, the desktop
// editor, and any future importer all go through here so the index never drifts
// from the files. Returns the path written.
//
// user/feedback memories go to GlobalDir (shared across projects);
// project/reference stay in the project-specific Dir.
// When a memory changes type (e.g. from project to user), it removes the old
// copy from the other directory.
func (s Store) Save(m Memory) (string, error) {
	if s.Dir == "" && s.GlobalDir == "" {
		return "", fmt.Errorf("memory store unavailable (no user config dir)")
	}
	name := slug(m.Name)
	if name == "" {
		return "", fmt.Errorf("memory needs a name")
	}
	dir := s.saveDir(NormalizeType(string(m.Type)))
	if dir == "" {
		return "", fmt.Errorf("memory store unavailable for type %q", m.Type)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path, err := safeJoin(dir, name+".md")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(render(m, name)), 0o644); err != nil {
		return "", err
	}
	if err := reindexIn(dir, name, m); err != nil {
		return path, err
	}
	// If this memory exists in another directory (e.g. was project type, now
	// user type), remove the stale copy from the other directory.
	for _, otherDir := range s.dirs() {
		if sameDir(otherDir, dir) {
			continue
		}
		if err := removeActiveMemoryInDir(otherDir, name); err != nil {
			return path, err
		}
	}
	return path, nil
}

// Archive removes a memory from the active store and moves its file under
// .archive/ for traceability. A missing file is not an error; the goal state
// (not active) already holds. It returns the archive path, or "" when no file
// existed to archive.
// When both GlobalDir and Dir exist, it archives from every directory the
// memory appears in (handles migration duplicates).
func (s Store) Archive(name string) (string, error) {
	if s.Dir == "" && s.GlobalDir == "" {
		return "", fmt.Errorf("memory store unavailable (no user config dir)")
	}
	name = slug(name)
	if name == "" {
		return "", fmt.Errorf("memory needs a name")
	}
	var lastPath string
	anyChange := false
	for _, dir := range s.dirs() {
		if dir == "" {
			continue
		}
		p, err := archiveInDir(dir, name)
		if err != nil {
			return "", err
		}
		if p != "" || indexContainsIn(dir, name) {
			anyChange = true
			if err := flushIndexIn(dir, indexLinesExceptIn(dir, name)); err != nil {
				return "", err
			}
		}
		if p != "" {
			lastPath = p
		}
	}
	if !anyChange && lastPath == "" {
		if suggestion := closestName(s.List(), name); suggestion != "" {
			return "", fmt.Errorf("memory %q not found — did you mean %q?", name, suggestion)
		}
	}
	return lastPath, nil
}

// Delete removes a memory from the active store and its MEMORY.md line — the
// model's `forget` path and the user's way to prune a stale fact. It archives
// the file instead of permanently deleting it so wrong memories remain
// traceable. A missing file is not an error; the goal state (gone) holds either
// way.
func (s Store) Delete(name string) error {
	_, err := s.Archive(name)
	return err
}

// List returns the saved memories parsed from their files (both Dir and
// GlobalDir merged), sorted by name. Used by `/memory` and the desktop memory
// panel. Files that fail to parse are skipped so one bad file never hides the
// rest.
func (s Store) List() []Memory {
	if s.Dir == "" && s.GlobalDir == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []Memory
	for _, dir := range s.dirs() {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || e.Name() == indexFile || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			if m, ok := loadMemory(filepath.Join(dir, e.Name())); ok {
				if m.Name == "" {
					m.Name = strings.TrimSuffix(e.Name(), ".md")
				}
				if seen[m.Name] {
					continue
				}
				seen[m.Name] = true
				out = append(out, m)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListArchived returns all memories that have been archived, sorted by archive
// time (newest first). Used by the memory panel for traceability.
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

// --- internal helpers ---

// safeJoin joins base and name and rejects paths that escape base (path
// traversal protection). Returns an absolute path or an error.
func safeJoin(base, name string) (string, error) {
	clean := filepath.Clean(filepath.Join(base, name))
	// On Windows, also normalize the volume/separator for comparison.
	if !strings.HasPrefix(clean, filepath.Clean(base)+string(os.PathSeparator)) && clean != filepath.Clean(base) {
		return "", fmt.Errorf("memory path escapes store: %s", name)
	}
	return clean, nil
}

// removeActiveMemoryInDir removes a memory from dir — archives the file, then
// flushes the index. Dir is left untouched if it doesn't contain the memory.
func removeActiveMemoryInDir(dir, name string) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	p, err := archiveInDir(dir, name)
	if err != nil {
		return err
	}
	if p != "" || indexContainsIn(dir, name) {
		return flushIndexIn(dir, indexLinesExceptIn(dir, name))
	}
	return nil
}

// archiveInDir moves a memory file from dir into dir/.archive/ with a
// timestamp prefix. Returns the archive path, or "" when the file doesn't
// exist in dir.
func archiveInDir(dir, name string) (string, error) {
	root, err := os.OpenRoot(dir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer root.Close()

	file := name + ".md"
	if _, err := root.Stat(file); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if err := root.MkdirAll(".archive", 0o755); err != nil {
		return "", err
	}
	dest, err := archivePath(name, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if err := root.Rename(file, ".archive/"+dest); err != nil {
		return "", err
	}
	return filepath.Join(dir, ".archive", dest), nil
}

// archivePath returns the archive filename: "<timestamp>-<name>.md".
func archivePath(name string, when time.Time) (string, error) {
	stamp := when.Format("20060102-150405.000")
	dest := stamp + "-" + name + ".md"
	if _, err := safeJoin(".archive", dest); err != nil {
		return "", err
	}
	return dest, nil
}

// archiveTimeFromName extracts the timestamp from an archive filename like
// "20060102-150405.000-name.md".
func archiveTimeFromName(name string) time.Time {
	const stampLen = len("20060102-150405.000")
	if len(name) <= stampLen || name[stampLen] != '-' {
		return time.Time{}
	}
	when, err := time.ParseInLocation("20060102-150405.000", name[:stampLen], time.UTC)
	if err != nil {
		return time.Time{}
	}
	return when
}

// indexContainsIn checks whether name appears in the MEMORY.md of dir.
func indexContainsIn(dir, name string) bool {
	b, err := os.ReadFile(filepath.Join(dir, indexFile))
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "("+name+".md)")
}

// render serializes a memory to frontmatter + body. The frontmatter mirrors the
// auto-memory shape (name / description / metadata.type) so the files are
// interchangeable with that ecosystem and re-readable by loadMemory.
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
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(m.Body))
	b.WriteString("\n")
	return b.String()
}

// indexLineRe matches a managed index line so reindex/Delete can target the line
// for one memory by its filename without disturbing the rest of a hand-edited
// MEMORY.md.
var indexLineRe = regexp.MustCompile(`\]\(([^)]+)\.md\)`)

// indexLinesExceptIn returns the managed MEMORY.md lines in dir keyed by filename
// stem, dropping the entry for name (a missing index → empty map).
func indexLinesExceptIn(dir, name string) map[string]string {
	existing, _ := os.ReadFile(filepath.Join(dir, indexFile))
	keep := map[string]string{}
	for _, line := range strings.Split(string(existing), "\n") {
		if mt := indexLineRe.FindStringSubmatch(line); mt != nil && mt[1] != name {
			keep[mt[1]] = strings.TrimRight(line, "\r")
		}
	}
	return keep
}

// flushIndexIn rewrites MEMORY.md in dir from the managed lines, sorted by
// filename.
func flushIndexIn(dir string, lines map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
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

// reindexIn rewrites the MEMORY.md line in dir for name, preserving every other
// managed line. The line is "- [<title>](<name>.md) — <description>"; title
// falls back to a de-kebabed name so the index reads as a label, never a bare
// slug.
func reindexIn(dir, name string, m Memory) error {
	lines := indexLinesExceptIn(dir, name)
	lines[name] = fmt.Sprintf("- [%s](%s.md) — %s", displayTitle(m.Title, name), name, oneLine(m.Description))
	return flushIndexIn(dir, lines)
}

// loadMemory parses one fact file back into a Memory. It tolerates the minimal
// frontmatter render writes; a file without frontmatter still loads with its
// body and a name derived from the filename.
func loadMemory(path string) (Memory, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Memory{}, false
	}
	info, _ := os.Stat(path)
	fm, body := splitFrontmatter(string(b))
	m := Memory{
		Name:        fm["name"],
		Title:       fm["title"],
		Description: fm["description"],
		Type:        NormalizeType(fm["type"]),
		Body:        strings.TrimSpace(body),
	}
	if info != nil {
		m.Mtime = info.ModTime()
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

// closestName returns the closest memory name to target via Levenshtein distance,
// or "" when no name is within a reasonable threshold (max distance = len(target)/2 + 1).
func closestName(memories []Memory, target string) string {
	if len(memories) == 0 || target == "" {
		return ""
	}
	best := ""
	bestDist := len(target)/2 + 1 // max allowed distance
	for _, m := range memories {
		d := levenshtein(target, m.Name)
		if d < bestDist {
			bestDist = d
			best = m.Name
		}
	}
	return best
}

// levenshtein computes the edit distance between a and b.
func levenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	n, m := len(ra), len(rb)
	// Use two-row DP to save memory.
	prev := make([]int, m+1)
	cur := make([]int, m+1)
	for j := 0; j <= m; j++ {
		prev[j] = j
	}
	for i := 1; i <= n; i++ {
		cur[0] = i
		for j := 1; j <= m; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[m]
}

func min3(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
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
