package codegraph

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ProjectMap is a lightweight, dependency-free project structure analyzer.
// It scans the workspace at boot time and produces a structured architecture
// block that gets injected into the cache-stable system prompt prefix.
//
// The output is additive markdown — it never replaces existing guidance, only
// appends a "live" project snapshot alongside the hand-written AGENTS.md etc.

// ProjectInfo carries the extracted project structure.
type ProjectInfo struct {
	Language     string    // "Go", "TypeScript", "Rust", etc.
	EntryPoint   string    // main.go / index.ts / src/main.rs
	Packages     []string  // key packages (internal/ names)
	CoreTypes    []string  // important struct/interface names
	DepsShort    []string  // <= 10 key dependencies from go.mod
	FileCount    int       // total source files (.go / .ts/.tsx / .rs) in the project
	LastModified time.Time // latest modtime across scanned files
}

// Analyze scans wsRoot (the project root) and returns a ProjectInfo.
// Analyze scans wsRoot (the project root) and returns a ProjectInfo.
// Returns zero-value ProjectInfo when the directory isn't recognizable.
func Analyze(wsRoot string) ProjectInfo {
	info := ProjectInfo{}
	var latestMod time.Time

	// Detect language and entry point
	if hasFile(wsRoot, "go.mod") {
		info.Language = "Go"
		info.EntryPoint = findEntryGo(wsRoot)
		info.DepsShort = extractKeyDeps(filepath.Join(wsRoot, "go.mod"))
		info.Packages = discoverPackages(wsRoot)
		info.CoreTypes = discoverCoreTypes(wsRoot)
		info.FileCount = countGoFiles(wsRoot)
		// Track latest modtime of go.mod
		if fi, err := os.Stat(filepath.Join(wsRoot, "go.mod")); err == nil {
			latestMod = fi.ModTime()
		}
	} else if hasFile(wsRoot, "Cargo.toml") {
		info.Language = "Rust"
		info.EntryPoint = "Cargo.toml (workspace)"
		info.DepsShort = extractKeyDeps(filepath.Join(wsRoot, "Cargo.toml"))
		info.Packages = discoverRustPackages(wsRoot)
		info.CoreTypes = discoverRustCoreTypes(wsRoot)
		info.FileCount = countRSFiles(wsRoot)
		if fi, err := os.Stat(filepath.Join(wsRoot, "Cargo.toml")); err == nil {
			latestMod = fi.ModTime()
		}
	} else if hasFile(wsRoot, "package.json") {
		info.Language = detectNodeLanguage(wsRoot)
		if hasFile(wsRoot, "tsconfig.json") {
			info.EntryPoint = findEntryTS(wsRoot)
		} else {
			info.EntryPoint = findEntryJS(wsRoot)
		}
		info.FileCount = countTSFiles(wsRoot)
	}

	// Track latest modtime across internal/ directories for change detection.
	if !hasFile(wsRoot, "go.mod") {
		// For non-Go projects, track package.json
		if fi, err := os.Stat(filepath.Join(wsRoot, "package.json")); err == nil {
			if fi.ModTime().After(latestMod) {
				latestMod = fi.ModTime()
			}
		}
	}
	internalDir := filepath.Join(wsRoot, "internal")
	if fi, err := os.Stat(internalDir); err == nil && fi.IsDir() {
		if fi.ModTime().After(latestMod) {
			latestMod = fi.ModTime()
		}
	}

	info.LastModified = latestMod
	return info
}

// Format renders the project map as a markdown block ready for prompt injection.
func (p ProjectInfo) Format() string {
	if p.Language == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Project Map (auto-detected)\n\n")

	b.WriteString(fmt.Sprintf("- **Language**: %s\n", p.Language))

	if p.EntryPoint != "" {
		b.WriteString(fmt.Sprintf("- **Entry**: `%s`\n", p.EntryPoint))
	}

	if p.FileCount > 0 {
		b.WriteString(fmt.Sprintf("- **Source Files**: %d\n", p.FileCount))
	}

	if !p.LastModified.IsZero() {
		b.WriteString(fmt.Sprintf("- **Last Modified**: %s\n", p.LastModified.Format("2006-01-02 15:04")))
	}

	if len(p.Packages) > 0 {
		b.WriteString("- **Packages**:\n")
		for _, pkg := range p.Packages {
			b.WriteString(fmt.Sprintf("  - `%s`\n", pkg))
		}
	}

	if len(p.CoreTypes) > 0 {
		b.WriteString("- **Core Types**:\n")
		for _, ct := range p.CoreTypes {
			b.WriteString(fmt.Sprintf("  - `%s`\n", ct))
		}
	}

	if len(p.DepsShort) > 0 {
		b.WriteString(fmt.Sprintf("- **Key Deps**: `%s`\n", strings.Join(p.DepsShort, "`, `")))
	}

	return strings.TrimRight(b.String(), "\n")
}

// Hash returns a content hash of the project info for change detection.
// Only the structural fields (packages, core types, deps, file count) affect
// the hash — LastModified is excluded because it changes every edit.
func (p ProjectInfo) Hash() string {
	h := fnv.New64a()
	h.Write([]byte(p.Language))
	h.Write([]byte(p.EntryPoint))
	for _, pkg := range p.Packages {
		h.Write([]byte(pkg))
	}
	for _, ct := range p.CoreTypes {
		h.Write([]byte(ct))
	}
	for _, dep := range p.DepsShort {
		h.Write([]byte(dep))
	}
	// FileCount as string
	h.Write([]byte(fmt.Sprintf("%d", p.FileCount)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Refresh checks whether the manifest or structure directory has changed since
// the last analysis. If nothing changed, returns old unchanged; otherwise
// performs a full re-analysis. Structure proxies: internal/ for Go, src/ for
// Node/TS. Projects without a recognized manifest always re-analyze.
func Refresh(wsRoot string, old ProjectInfo) ProjectInfo {
	// Go: go.mod + internal/
	if hasFile(wsRoot, "go.mod") {
		fi, err := os.Stat(filepath.Join(wsRoot, "go.mod"))
		if err == nil && !fi.ModTime().After(old.LastModified) {
			internalDir := filepath.Join(wsRoot, "internal")
			if fi2, err := os.Stat(internalDir); err == nil && fi2.IsDir() {
				if !fi2.ModTime().After(old.LastModified) {
					return old
				}
			}
		}
		return Analyze(wsRoot)
	}
	// Node/TS: package.json + src/
	if hasFile(wsRoot, "package.json") {
		fi, err := os.Stat(filepath.Join(wsRoot, "package.json"))
		if err == nil && !fi.ModTime().After(old.LastModified) {
			srcDir := filepath.Join(wsRoot, "src")
			if fi2, err := os.Stat(srcDir); err == nil && fi2.IsDir() {
				if !fi2.ModTime().After(old.LastModified) {
					return old
				}
			}
		}
		return Analyze(wsRoot)
	}
	// Rust: Cargo.toml + src/
	if hasFile(wsRoot, "Cargo.toml") {
		fi, err := os.Stat(filepath.Join(wsRoot, "Cargo.toml"))
		if err == nil && !fi.ModTime().After(old.LastModified) {
			srcDir := filepath.Join(wsRoot, "src")
			if fi2, err := os.Stat(srcDir); err == nil && fi2.IsDir() {
				if !fi2.ModTime().After(old.LastModified) {
					return old
				}
			}
		}
		return Analyze(wsRoot)
	}
	// Unrecognized manifest — no incremental detection; full re-analysis
	return Analyze(wsRoot)
}

// --- helpers ---------------------------------------------------------------

func hasFile(dir, name string) bool {
	fi, err := os.Stat(filepath.Join(dir, name))
	return err == nil && !fi.IsDir()
}

// findEntryGo locates the main package entry point.
func findEntryGo(root string) string {
	// Walk cmd/ directories looking for main.go
	cmdDir := filepath.Join(root, "cmd")
	if fi, err := os.Stat(cmdDir); err == nil && fi.IsDir() {
		entries, _ := os.ReadDir(cmdDir)
		for _, e := range entries {
			if e.IsDir() {
				if hasFile(filepath.Join(cmdDir, e.Name()), "main.go") {
					return "cmd/" + e.Name() + "/main.go"
				}
			}
		}
	}

	// Fallback: root main.go
	if hasFile(root, "main.go") {
		return "main.go"
	}

	// Cmd subdirectory found but no main.go there — list dirs that exist
	if fi, err := os.Stat(cmdDir); err == nil && fi.IsDir() {
		return "cmd/ (no main.go found)"
	}

	return ""
}

// findEntryTS locates TypeScript entry point (main entry in package.json or src/).
func findEntryTS(root string) string {
	// Try src/main.ts, src/index.ts, src/App.tsx, etc.
	candidates := []string{"src/main.ts", "src/main.tsx", "src/index.ts", "src/index.tsx", "src/App.tsx"}
	for _, c := range candidates {
		if hasFile(root, c) {
			return c
		}
	}
	// Check package.json "main" field
	return "package.json (check main field)"
}

func findEntryJS(root string) string {
	candidates := []string{"src/index.js", "src/main.js", "index.js", "src/index.ts", "src/main.ts"}
	for _, c := range candidates {
		if hasFile(root, c) {
			return c
		}
	}
	return "index.js"
}

func detectNodeLanguage(root string) string {
	if hasFile(root, "tsconfig.json") {
		return "TypeScript"
	}
	return "JavaScript"
}

// extractKeyDeps reads go.mod and returns the first N direct dependencies.
func extractKeyDeps(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var deps []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		// Go: require line
		if strings.HasPrefix(line, "require (") || line == ")" || line == "" {
			continue
		}
		if strings.HasPrefix(line, "require ") {
			continue
		}
		// Match "pkg v1.2.3" within require block
		parts := strings.Fields(line)
		if len(parts) >= 2 && !strings.Contains(line, "// indirect") {
			// Extract short name from path
			pkg := parts[0]
			short := shortenPkg(pkg)
			if short != "" && !isStdLib(short) {
				deps = append(deps, short)
			}
		}
	}
	if len(deps) > 10 {
		deps = deps[:10]
	}
	return deps
}

func shortenPkg(pkg string) string {
	parts := strings.Split(pkg, "/")
	if len(parts) >= 3 {
		return parts[0] + "/" + parts[1] + "/" + parts[2]
	}
	return pkg
}

var stdLibs = map[string]bool{
	"fmt": true, "os": true, "strings": true, "io": true, "net": true,
	"http": true, "json": true, "time": true, "sync": true, "context": true,
	"regexp": true, "sort": true, "path": true, "flag": true, "log": true,
	"math": true, "bytes": true, "encoding": true, "crypto": true,
	"errors": true, "reflect": true, "strconv": true, "unicode": true,
	"bufio": true, "compress": true, "container": true, "database": true,
	"embed": true, "hash": true, "html": true, "image": true, "index": true,
	"internal": true, "mime": true, "net/http": true, "os/exec": true,
	"path/filepath": true, "runtime": true, "testing": true, "text": true,
}

func isStdLib(name string) bool {
	return stdLibs[name]
}

// discoverPackages scans internal/ for packages (Go).
func discoverPackages(root string) []string {
	internalDir := filepath.Join(root, "internal")
	fi, err := os.Stat(internalDir)
	if err != nil || !fi.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return nil
	}

	var pkgs []string
	for _, e := range entries {
		if e.IsDir() {
			// Check it has at least one .go file
			hasGo := false
			sub, _ := os.ReadDir(filepath.Join(internalDir, e.Name()))
			for _, subEntry := range sub {
				if !subEntry.IsDir() && strings.HasSuffix(subEntry.Name(), ".go") {
					hasGo = true
					break
				}
			}
			if hasGo {
				pkgs = append(pkgs, e.Name())
			}
		}
	}

	sort.Strings(pkgs)
	return pkgs
}

// discoverCoreTypes scans all .go files under internal/ for prominent type
// definitions. Reads only the first 40 lines of each file to keep startup fast.
func discoverCoreTypes(root string) []string {
	internalDir := filepath.Join(root, "internal")
	fi, err := os.Stat(internalDir)
	if err != nil || !fi.IsDir() {
		return nil
	}

	var types []string
	filepath.Walk(internalDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(fi.Name(), ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(b), "\n")
		limit := 40
		if len(lines) < limit {
			limit = len(lines)
		}
		for _, line := range lines[:limit] {
			trimmed := strings.TrimSpace(line)
			// Match "type X struct {", "type X interface {"
			if strings.HasPrefix(trimmed, "type ") && (strings.HasSuffix(trimmed, "{") || strings.Contains(trimmed, " struct") || strings.Contains(trimmed, " interface")) {
				parts := strings.Fields(trimmed)
				if len(parts) >= 2 {
					name := strings.TrimSuffix(parts[1], ",")
					if name != "" && !strings.HasPrefix(name, "//") {
						types = append(types, name+" ("+filepath.Base(filepath.Dir(path))+")")
					}
				}
			}
		}
		return nil
	})

	// Deduplicate
	seen := map[string]bool{}
	var unique []string
	for _, t := range types {
		if !seen[t] {
			seen[t] = true
			unique = append(unique, t)
		}
	}
	unique = rankCoreTypes(unique, internalDir, ".go")
	if len(unique) > 15 {
		unique = unique[:15]
	}
	return unique
}

// countGoFiles counts the number of .go files in the workspace.
func countGoFiles(root string) int {
	var count int
	filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		// Skip vendor, node_modules, .git
		if strings.Contains(path, "vendor") || strings.Contains(path, "node_modules") || strings.Contains(path, ".git") {
			return nil
		}
		if strings.HasSuffix(fi.Name(), ".go") {
			count++
		}
		return nil
	})
	return count
}

// countTSFiles counts TypeScript source files (.ts, .tsx).
func countTSFiles(root string) int {
	var count int
	filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if strings.Contains(path, "node_modules") || strings.Contains(path, ".git") {
			return nil
		}
		if strings.HasSuffix(fi.Name(), ".ts") || strings.HasSuffix(fi.Name(), ".tsx") {
			count++
		}
		return nil
	})
	return count
}

// countRSFiles counts Rust source files (.rs), skipping build artifacts
// (target/) and VCS/dependency directories.
func countRSFiles(root string) int {
	var count int
	filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if strings.Contains(path, "target") || strings.Contains(path, ".git") || strings.Contains(path, "node_modules") {
			return nil
		}
		if strings.HasSuffix(fi.Name(), ".rs") {
			count++
		}
		return nil
	})
	return count
}

// discoverRustPackages returns the module structure under src/: direct
// subdirectories containing .rs files, plus top-level module files
// (excluding the crate roots main.rs/lib.rs).
func discoverRustPackages(root string) []string {
	srcDir := filepath.Join(root, "src")
	fi, err := os.Stat(srcDir)
	if err != nil || !fi.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil
	}
	var pkgs []string
	for _, e := range entries {
		if e.IsDir() {
			if dirHasRSFiles(filepath.Join(srcDir, e.Name())) {
				pkgs = append(pkgs, e.Name())
			}
		} else if strings.HasSuffix(e.Name(), ".rs") {
			name := strings.TrimSuffix(e.Name(), ".rs")
			if name != "main" && name != "lib" {
				pkgs = append(pkgs, name)
			}
		}
	}
	sort.Strings(pkgs)
	return pkgs
}

func dirHasRSFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".rs") {
			return true
		}
	}
	return false
}

// discoverRustCoreTypes scans .rs files under src/ for prominent public type
// definitions (struct/enum/trait/type). Reads only the first 40 lines of each
// file to keep startup fast.
func discoverRustCoreTypes(root string) []string {
	srcDir := filepath.Join(root, "src")
	fi, err := os.Stat(srcDir)
	if err != nil || !fi.IsDir() {
		return nil
	}
	var types []string
	filepath.Walk(srcDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(fi.Name(), ".rs") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(b), "\n")
		limit := 40
		if len(lines) < limit {
			limit = len(lines)
		}
		for _, line := range lines[:limit] {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "pub") {
				continue
			}
			kw := rustTypeKeyword(trimmed)
			if kw == "" {
				continue
			}
			parts := strings.Fields(trimmed)
			for i, p := range parts {
				if p != kw || i+1 >= len(parts) {
					continue
				}
				name := strings.TrimSuffix(parts[i+1], ",")
				if name != "" {
					types = append(types, name+" ("+filepath.Base(filepath.Dir(path))+")")
				}
				break
			}
		}
		return nil
	})
	// Deduplicate
	seen := map[string]bool{}
	var unique []string
	for _, t := range types {
		if !seen[t] {
			seen[t] = true
			unique = append(unique, t)
		}
	}
	unique = rankCoreTypes(unique, srcDir, ".rs")
	if len(unique) > 15 {
		unique = unique[:15]
	}
	return unique
}

// rankCoreTypes reorders "Name (dir)" entries by descending whole-identifier
// reference frequency across the scanned tree — Aider repo-map 蒸馏：地图展示
// 被引用最多的类型，而非扫描顺序的前 N 个。同频次按名称字母序保证确定性。
func rankCoreTypes(items []string, scanDir, ext string) []string {
	if len(items) == 0 {
		return nil
	}
	counts := make(map[string]int, len(items))
	for _, it := range items {
		counts[typeNameOf(it)] = 0
	}
	filepath.Walk(scanDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(fi.Name(), ext) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(b)
		for name := range counts {
			counts[name] += countIdentifier(content, name)
		}
		return nil
	})
	sort.Slice(items, func(i, j int) bool {
		ni, nj := typeNameOf(items[i]), typeNameOf(items[j])
		if counts[ni] != counts[nj] {
			return counts[ni] > counts[nj]
		}
		return ni < nj
	})
	return items
}

// typeNameOf extracts the identifier from a "Name (dir)" entry.
func typeNameOf(entry string) string {
	fields := strings.Fields(entry)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// countIdentifier counts occurrences of name as a whole identifier in content
// (letter/digit/underscore boundaries), ignoring substring matches like
// "Handler" inside "HTTPHandler".
func countIdentifier(content, name string) int {
	n := 0
	i := 0
	for i < len(content) {
		j := strings.Index(content[i:], name)
		if j < 0 {
			break
		}
		start := i + j
		end := start + len(name)
		if (start == 0 || !isIdentByte(content[start-1])) &&
			(end >= len(content) || !isIdentByte(content[end])) {
			n++
		}
		i = end
	}
	return n
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// rustTypeKeyword returns the type keyword (struct/enum/trait/type) declared
// on a pub line, or "" when the line declares something else (fn, mod, use...).
func rustTypeKeyword(trimmed string) string {
	for _, kw := range []string{"struct", "enum", "trait", "type"} {
		if strings.Contains(trimmed, " "+kw+" ") {
			return kw
		}
	}
	return ""
}
