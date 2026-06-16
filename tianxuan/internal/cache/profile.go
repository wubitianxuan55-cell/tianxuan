package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Profile holds a lightweight project snapshot built at session start.
// It is injected into the Context domain so the agent has structural
// awareness without spending turns exploring the file tree.
type Profile struct {
	Language     string
	Module       string
	EntryPoints  []string
	Packages     int
	TestFiles    int
	TotalFiles   int
	Dependencies []string
	TopDirs      []string
}

// Scan walks the project root and builds a Profile. Best-effort: a missing
// go.mod or unreadable directory merely produces a thinner profile. Target
// time is < 2s even on large repos (depth-limited).
func (p *Profile) Scan(root string) {
	if root == "" {
		root = "."
	}
	p.Language = "Go"
	p.readModule(root)
	p.scanTree(root)
}

// Render returns a compact Markdown block suitable for the Context domain.
// Returns empty string when the profile carries no useful information.
func (p *Profile) Render() string {
	if p.TotalFiles == 0 && p.Module == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Project Profile\n\n")

	if p.Module != "" {
		fmt.Fprintf(&b, "- **Module:** %s\n", p.Module)
	}
	fmt.Fprintf(&b, "- **Language:** %s\n", p.Language)

	if len(p.EntryPoints) > 0 {
		fmt.Fprintf(&b, "- **Entry points:** %s\n", strings.Join(p.EntryPoints, ", "))
	}

	if p.Packages > 0 {
		fmt.Fprintf(&b, "- **Packages:** %d\n", p.Packages)
	}

	fmt.Fprintf(&b, "- **Files:** %d Go files", p.TotalFiles)
	if p.TestFiles > 0 {
		fmt.Fprintf(&b, " (%d test files)", p.TestFiles)
	}
	b.WriteString("\n")

	if len(p.Dependencies) > 0 {
		deps := p.Dependencies
		if len(deps) > 10 {
			deps = deps[:10]
		}
		fmt.Fprintf(&b, "- **Dependencies:** %s\n", strings.Join(deps, ", "))
	}

	if len(p.TopDirs) > 0 {
		dirs := p.TopDirs
		if len(dirs) > 12 {
			dirs = dirs[:12]
		}
		fmt.Fprintf(&b, "- **Key directories:** %s\n", strings.Join(dirs, ", "))
	}

	return b.String()
}

// readModule parses go.mod for the module name and direct dependencies.
func (p *Profile) readModule(root string) {
	path := filepath.Join(root, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	inRequire := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "module ") {
			p.Module = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			continue
		}
		if line == "require (" {
			inRequire = true
			continue
		}
		if inRequire {
			if line == ")" {
				inRequire = false
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				p.Dependencies = append(p.Dependencies, parts[0])
			}
		} else if strings.HasPrefix(line, "require ") {
			parts := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(parts) >= 1 {
				p.Dependencies = append(p.Dependencies, parts[0])
			}
		}
	}
}

// scanTree walks the project tree (depth-limited) and collects file counts,
// entry points, and directory names.
func (p *Profile) scanTree(root string) {
	const maxDepth = 8 // skip dirs deeper than this to bound startup time
	dirs := map[string]bool{}
	seenPackages := map[string]bool{}

	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden, vendor, dependency, and large-data dirs
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" ||
				name == "third_party" || name == "testdata" || name == ".git" {
				return filepath.SkipDir
			}
			// Depth limit: skip directories nested deeper than maxDepth from root.
			rel, _ := filepath.Rel(root, path)
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= maxDepth {
				return filepath.SkipDir
			}
			// Track top-level dirs
			if rel != "." && !strings.Contains(rel, string(filepath.Separator)) {
				dirs[rel] = true
			}
			return nil
		}

		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		p.TotalFiles++
		if strings.HasSuffix(d.Name(), "_test.go") {
			p.TestFiles++
			return nil
		}

		// Track packages (one per directory with non-test .go files)
		dir := filepath.Dir(path)
		rel, _ := filepath.Rel(root, dir)
		if rel != "." {
			seenPackages[rel] = true
		}
		return nil
	})

	// Find entry points: any main.go or cmd/*/main.go
	if entries, _ := filepath.Glob(filepath.Join(root, "main.go")); len(entries) > 0 {
		p.EntryPoints = append(p.EntryPoints, "main.go")
	}
	cmdEntries, _ := filepath.Glob(filepath.Join(root, "cmd", "*", "main.go"))
	for _, e := range cmdEntries {
		rel, _ := filepath.Rel(root, e)
		p.EntryPoints = append(p.EntryPoints, filepath.ToSlash(rel))
	}

	p.Packages = len(seenPackages)

	// Sort dirs
	for d := range dirs {
		p.TopDirs = append(p.TopDirs, d)
	}
	sort.Strings(p.TopDirs)
}
