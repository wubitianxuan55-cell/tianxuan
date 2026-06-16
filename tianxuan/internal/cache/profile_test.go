package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileScanFindsGoModule(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "internal", "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "internal", "pkg", "x.go"), []byte("package pkg\n"), 0644)
	os.WriteFile(filepath.Join(dir, "internal", "pkg", "x_test.go"), []byte("package pkg_test\n"), 0644)

	var p Profile
	p.Scan(dir)

	if p.Module != "example.com/test" {
		t.Errorf("Module = %q, want example.com/test", p.Module)
	}
	if len(p.EntryPoints) == 0 {
		t.Error("EntryPoints should include main.go")
	}
	if p.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3 (main + x + x_test)", p.TotalFiles)
	}
	if p.TestFiles != 1 {
		t.Errorf("TestFiles = %d, want 1", p.TestFiles)
	}
}

func TestProfileRenderIncludesKeyInfo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\nrequire github.com/foo/bar v1.0\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "internal", "core"), 0755)
	os.WriteFile(filepath.Join(dir, "internal", "core", "app.go"), []byte("package core\n"), 0644)

	var p Profile
	p.Scan(dir)
	out := p.Render()

	if !strings.Contains(out, "example.com/app") {
		t.Errorf("Render missing module: %s", out)
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("Render missing entry point: %s", out)
	}
	if !strings.Contains(out, "Go") {
		t.Errorf("Render missing language: %s", out)
	}
	if !strings.Contains(out, "foo/bar") {
		t.Errorf("Render missing dependency: %s", out)
	}
}

func TestProfileEmptyProject(t *testing.T) {
	dir := t.TempDir()
	var p Profile
	p.Scan(dir)

	if p.TotalFiles != 0 {
		t.Errorf("empty dir should have 0 files, got %d", p.TotalFiles)
	}
	if p.Render() != "" {
		t.Errorf("empty project should render empty: %s", p.Render())
	}
}
