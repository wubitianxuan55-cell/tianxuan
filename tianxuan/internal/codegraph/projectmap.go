package codegraph

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectMap is a lightweight, dependency-free project structure analyzer.
// It scans the workspace at boot time and produces a structured architecture
// block that gets injected into the cache-stable system prompt prefix.
//
// The output is additive markdown — it never replaces existing guidance, only
// appends a "live" project snapshot alongside the hand-written AGENTS.md etc.

// ProjectInfo carries the extracted project structure.
type ProjectInfo struct {
	Language   string   // "Go", "TypeScript", "Rust", etc.
	EntryPoint string   // main.go / index.ts / src/main.rs
	Packages   []string // key packages (internal/ names)
	CoreTypes  []string // important struct/interface names
	DepsShort  []string // <= 10 key dependencies from go.mod
}

// Analyze scans wsRoot (the project root) and returns a ProjectInfo.
// Returns zero-value ProjectInfo when the directory isn't recognizable.
func Analyze(wsRoot string) ProjectInfo {
	info := ProjectInfo{}

	// Detect language and entry point
	if hasFile(wsRoot, "go.mod") {
		info.Language = "Go"
		info.EntryPoint = findEntryGo(wsRoot)
		info.DepsShort = extractKeyDeps(filepath.Join(wsRoot, "go.mod"))
		info.Packages = discoverPackages(wsRoot)
		info.CoreTypes = discoverCoreTypes(wsRoot)
	} else if hasFile(wsRoot, "Cargo.toml") {
		info.Language = "Rust"
		info.EntryPoint = "Cargo.toml (workspace)"
		info.DepsShort = extractKeyDeps(filepath.Join(wsRoot, "Cargo.toml"))
	} else if hasFile(wsRoot, "package.json") {
		info.Language = detectNodeLanguage(wsRoot)
		if hasFile(wsRoot, "tsconfig.json") {
			info.EntryPoint = findEntryTS(wsRoot)
		} else {
			info.EntryPoint = findEntryJS(wsRoot)
		}
	}

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
				mainGo := filepath.Join(cmdDir, e.Name(), "main.go")
				if hasFile(filepath.Join(cmdDir, e.Name()), "main.go") {
					_ = mainGo
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

// discoverCoreTypes scans key packages for prominent type definitions.
// It only reads the first 40 lines of key files to keep startup fast.
func discoverCoreTypes(root string) []string {
	// Key files that likely define the core types
	keyFiles := []string{
		"internal/control/controller.go",
		"internal/agent/agent.go",
		"internal/tool/tool.go",
	}

	var types []string
	for _, rel := range keyFiles {
		path := filepath.Join(root, rel)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
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
						types = append(types, name)
					}
				}
			}
		}
	}

	// Deduplicate
	seen := map[string]bool{}
	var unique []string
	for _, t := range types {
		if !seen[t] {
			seen[t] = true
			unique = append(unique, t)
		}
	}
	if len(unique) > 15 {
		unique = unique[:15]
	}
	return unique
}
