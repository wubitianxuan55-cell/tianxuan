package codegraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// chtimesPast fixes a file/dir modtime to (now - hours) so later writes
// deterministically advance it on any filesystem.
func chtimesPast(t *testing.T, path string, hours int) {
	t.Helper()
	past := time.Now().Add(-time.Duration(hours) * time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
}

func writeFileT(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRefresh_NodeIncremental locks the Node/TS incremental semantics:
// package.json + src/ unchanged → reuse cache; src/ change → re-analyze;
// root-level file change → no re-analysis (src/ is the structure proxy).
func TestRefresh_NodeIncremental(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "package.json"), `{"name":"demo","main":"src/index.ts"}`)
	writeFileT(t, filepath.Join(dir, "tsconfig.json"), `{"compilerOptions":{"strict":true}}`)
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(srcDir, "index.ts"), "export const a = 1;\n")

	// package.json 比 src/ 新：LastModified 以 package.json 为准，src 更旧才可增量。
	chtimesPast(t, filepath.Join(dir, "package.json"), 2)
	chtimesPast(t, srcDir, 3)

	first := Analyze(dir)
	if first.FileCount != 1 {
		t.Fatalf("expected 1 TS file, got %d", first.FileCount)
	}

	// 未变化 → 返回缓存值
	got := Refresh(dir, first)
	if got.FileCount != 1 || got.Language != "TypeScript" {
		t.Fatalf("unchanged node project should reuse cached info, got %+v", got)
	}

	// src/ 下新增文件 → 触发重扫
	writeFileT(t, filepath.Join(srcDir, "extra.ts"), "export const b = 2;\n")
	got = Refresh(dir, first)
	if got.FileCount != 2 {
		t.Fatalf("new src file should trigger re-analysis (FileCount=2), got %d", got.FileCount)
	}

	// src/ 外新增文件 → 不触发重扫（增量语义：src/ 是结构代理）
	chtimesPast(t, srcDir, 3) // 固定 src modtime 回到基准之前，模拟增量窗口
	writeFileT(t, filepath.Join(dir, "root.ts"), "export const c = 3;\n")
	got = Refresh(dir, got)
	if got.FileCount != 2 {
		t.Fatalf("root-level file change should NOT re-analyze, got %d", got.FileCount)
	}
}

// TestRefresh_GoIncremental locks the Go incremental semantics already present
// in Refresh: go.mod + internal/ unchanged → reuse cache; internal/ change → re-analyze.
func TestRefresh_GoIncremental(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "go.mod"), "module example.com/x\n\ngo 1.26\n")
	pkgDir := filepath.Join(dir, "internal", "pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(pkgDir, "x.go"), "package pkg\n\ntype Foo struct{}\n")
	internalDir := filepath.Join(dir, "internal")

	// internal/ 比 go.mod 新：LastModified 以 internal 为准。
	chtimesPast(t, filepath.Join(dir, "go.mod"), 3)
	chtimesPast(t, internalDir, 2)

	first := Analyze(dir)
	if first.FileCount != 1 {
		t.Fatalf("expected 1 go file, got %d", first.FileCount)
	}

	// 未变化 → 返回缓存值
	got := Refresh(dir, first)
	if got.FileCount != 1 {
		t.Fatalf("unchanged go project should reuse cached info, got %d", got.FileCount)
	}

	// internal/ 下新增包（创建直接子目录 → internal modtime 更新）→ 触发重扫
	if err := os.MkdirAll(filepath.Join(dir, "internal", "pkg2"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(dir, "internal", "pkg2", "z.go"), "package pkg2\n\ntype Bar struct{}\n")
	got = Refresh(dir, first)
	if got.FileCount != 2 {
		t.Fatalf("new internal package should trigger re-analysis (FileCount=2), got %d", got.FileCount)
	}
}

// TestRefresh_RustIncremental locks the Rust incremental semantics:
// Cargo.toml + src/ unchanged → reuse cache; src/ change → re-analyze;
// root-level file change → no re-analysis (src/ is the structure proxy).
func TestRefresh_RustIncremental(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "Cargo.toml"), "[package]\nname = \"demo\"\nversion = \"0.1.0\"\n")
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(srcDir, "main.rs"), "fn main() {}\n")

	// Cargo.toml 比 src/ 新：LastModified 以 Cargo.toml 为准，src 更旧才可增量。
	chtimesPast(t, filepath.Join(dir, "Cargo.toml"), 2)
	chtimesPast(t, srcDir, 3)

	first := Analyze(dir)
	if first.Language != "Rust" {
		t.Fatalf("expected Rust language, got %q", first.Language)
	}
	if first.FileCount != 1 {
		t.Fatalf("expected 1 rs file, got %d", first.FileCount)
	}

	// 未变化 → 返回缓存值
	got := Refresh(dir, first)
	if got.FileCount != 1 || got.Language != "Rust" {
		t.Fatalf("unchanged rust project should reuse cached info, got %+v", got)
	}

	// src/ 下新增文件 → 触发重扫
	writeFileT(t, filepath.Join(srcDir, "lib.rs"), "pub fn f() {}\n")
	got = Refresh(dir, first)
	if got.FileCount != 2 {
		t.Fatalf("new src file should trigger re-analysis (FileCount=2), got %d", got.FileCount)
	}

	// src/ 外新增文件 → 不触发重扫（增量语义：src/ 是结构代理）
	chtimesPast(t, srcDir, 3) // 固定 src modtime 回到基准之前，模拟增量窗口
	writeFileT(t, filepath.Join(dir, "build.rs"), "fn main() {}\n")
	got = Refresh(dir, got)
	if got.FileCount != 2 {
		t.Fatalf("root-level file change should NOT re-analyze, got %d", got.FileCount)
	}
}

// TestAnalyze_RustStructure locks Rust project-map structure support:
// src/ subdirectories and top-level module files surface as Packages,
// public type definitions surface as CoreTypes — mirroring the Go support.
func TestAnalyze_RustStructure(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "Cargo.toml"), "[package]\nname = \"demo\"\nversion = \"0.1.0\"\n")
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "parser"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(srcDir, "main.rs"), "fn main() {}\n")
	writeFileT(t, filepath.Join(srcDir, "lib.rs"), "pub struct Engine {}\npub trait Runner {}\n")
	writeFileT(t, filepath.Join(srcDir, "lexer.rs"), "pub enum TokenKind {}\n")
	writeFileT(t, filepath.Join(srcDir, "parser", "mod.rs"), "pub struct Ast {}\npub fn parse() {}\n")

	info := Analyze(dir)
	if info.Language != "Rust" {
		t.Fatalf("expected Rust language, got %q", info.Language)
	}
	wantPkgs := []string{"lexer", "parser"}
	if len(info.Packages) != len(wantPkgs) {
		t.Fatalf("expected %d packages, got %v", len(wantPkgs), info.Packages)
	}
	for i, want := range wantPkgs {
		if info.Packages[i] != want {
			t.Errorf("package[%d] = %q, want %q (full: %v)", i, info.Packages[i], want, info.Packages)
		}
	}

	got := map[string]bool{}
	for _, ct := range info.CoreTypes {
		got[ct] = true
	}
	for _, want := range []string{"Engine (src)", "Runner (src)", "TokenKind (src)", "Ast (parser)"} {
		if !got[want] {
			t.Errorf("core type %q missing: %v", want, info.CoreTypes)
		}
	}
	if got["parse (src)"] {
		t.Errorf("pub fn must not be collected as a core type: %v", info.CoreTypes)
	}
}

// TestCoreTypesRankedByReference distills Aider's repo-map insight: the map
// should surface the most-referenced types, not the first N in scan order.
// Alpha is defined late (internal/e) but referenced from many files, so it
// must rank above Zeta which is defined first but barely used.
func TestCoreTypesRankedByReference(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "go.mod"), "module example.com/x\n\ngo 1.26\n")
	for _, pkg := range []string{"a", "b", "c", "d", "e"} {
		if err := os.MkdirAll(filepath.Join(dir, "internal", pkg), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFileT(t, filepath.Join(dir, "internal", "a", "a.go"), "package a\n\ntype Zeta struct{}\n")
	writeFileT(t, filepath.Join(dir, "internal", "e", "e.go"), "package e\n\ntype Alpha struct{}\n")
	for _, pkg := range []string{"b", "c", "d"} {
		var b strings.Builder
		b.WriteString("package " + pkg + "\n\n")
		for i := 0; i < 5; i++ {
			b.WriteString("var _ = e.Alpha{}\n")
		}
		writeFileT(t, filepath.Join(dir, "internal", pkg, pkg+".go"), b.String())
	}

	info := Analyze(dir)
	if len(info.CoreTypes) < 2 {
		t.Fatalf("expected at least 2 core types, got %v", info.CoreTypes)
	}
	if info.CoreTypes[0] != "Alpha (e)" {
		t.Errorf("top core type should be Alpha (e) — most referenced — got %v", info.CoreTypes)
	}
	if info.CoreTypes[1] != "Zeta (a)" {
		t.Errorf("second core type should be Zeta (a), got %v", info.CoreTypes)
	}
}
