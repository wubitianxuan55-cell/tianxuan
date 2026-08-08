package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---- parsing --------------------------------------------------------------

func TestParsePatchAddFile(t *testing.T) {
	patch := "*** Begin Patch\n*** Add File: new.txt\n+hello\n+world\n*** End Patch"
	hs, err := parsePatch(patch)
	if err != nil {
		t.Fatalf("parsePatch: %v", err)
	}
	if len(hs) != 1 || hs[0].kind != "add" || hs[0].path != "new.txt" {
		t.Fatalf("hunks = %+v, want single add hunk for new.txt", hs)
	}
	if hs[0].contents != "hello\nworld\n" {
		t.Fatalf("contents = %q, want %q", hs[0].contents, "hello\nworld\n")
	}
}

func TestParsePatchDeleteFile(t *testing.T) {
	patch := "*** Begin Patch\n*** Delete File: old.txt\n*** End Patch"
	hs, err := parsePatch(patch)
	if err != nil {
		t.Fatalf("parsePatch: %v", err)
	}
	if len(hs) != 1 || hs[0].kind != "delete" || hs[0].path != "old.txt" {
		t.Fatalf("hunks = %+v, want single delete hunk for old.txt", hs)
	}
}

func TestParsePatchUpdateFile(t *testing.T) {
	patch := "*** Begin Patch\n" +
		"*** Update File: a.txt\n" +
		"@@ func main()\n" +
		" line1\n" +
		"-old line\n" +
		"+new line\n" +
		" line2\n" +
		"*** End of File\n" +
		"*** End Patch"
	hs, err := parsePatch(patch)
	if err != nil {
		t.Fatalf("parsePatch: %v", err)
	}
	if len(hs) != 1 || hs[0].kind != "update" || hs[0].path != "a.txt" {
		t.Fatalf("hunks = %+v, want single update hunk for a.txt", hs)
	}
	ch := hs[0].chunks
	if len(ch) != 1 {
		t.Fatalf("chunks = %+v, want 1 chunk", ch)
	}
	if ch[0].contextText != "func main()" {
		t.Fatalf("contextText = %q, want %q", ch[0].contextText, "func main()")
	}
	want := []changeLine{
		{kind: "ctx", text: "line1"},
		{kind: "old", text: "old line"},
		{kind: "new", text: "new line"},
		{kind: "ctx", text: "line2"},
	}
	if len(ch[0].lines) != len(want) {
		t.Fatalf("lines = %+v, want %+v", ch[0].lines, want)
	}
	for i := range want {
		if ch[0].lines[i] != want[i] {
			t.Fatalf("lines[%d] = %+v, want %+v", i, ch[0].lines[i], want[i])
		}
	}
	if !ch[0].atEOF {
		t.Fatal("chunk should be marked atEOF")
	}
}

func TestParsePatchBoundaries(t *testing.T) {
	if _, err := parsePatch(""); err == nil {
		t.Fatal("empty patch should fail")
	}
	if _, err := parsePatch("*** Update File: a.txt\n*** End Patch"); err == nil {
		t.Fatal("missing Begin Patch should fail")
	}
	if _, err := parsePatch("*** Begin Patch\n*** Update File: a.txt\n"); err == nil {
		t.Fatal("missing End Patch should fail")
	}
}

func TestParsePatchEmptyUpdateHunk(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: a.txt\n*** End Patch"
	_, err := parsePatch(patch)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v, want empty-update error with line number", err)
	}
}

func TestParsePatchUnknownLine(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: a.txt\nnot a patch line\n*** End Patch"
	_, err := parsePatch(patch)
	if err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("err = %v, want unknown-line error with line number", err)
	}
}

func TestParsePatchChunkWithoutDeletions(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: a.txt\n+only addition\n*** End Patch"
	_, err := parsePatch(patch)
	if err == nil || !strings.Contains(err.Error(), "deletion") {
		t.Fatalf("err = %v, want chunk-without-deletions error", err)
	}
}

func TestParsePatchLenientWhitespace(t *testing.T) {
	patch := "  *** Begin Patch  \n*** Add File: new.txt\n+hello\n*** End Patch  "
	hs, err := parsePatch(patch)
	if err != nil {
		t.Fatalf("parsePatch: %v", err)
	}
	if len(hs) != 1 || hs[0].path != "new.txt" {
		t.Fatalf("hunks = %+v, want add hunk for new.txt (leading/trailing space tolerated)", hs)
	}
}

// ---- execution ------------------------------------------------------------

func runApplyPatch(t *testing.T, dir, patch string) (string, error) {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(oldWD)
	args, err := json.Marshal(map[string]string{"patch": patch})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	var ap applyPatch
	return ap.Execute(context.Background(), args)
}

func TestApplyPatchToolAddFile(t *testing.T) {
	dir := t.TempDir()
	out, err := runApplyPatch(t, dir, "*** Begin Patch\n*** Add File: new.txt\n+hello\n+world\n*** End Patch")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "new.txt") {
		t.Fatalf("output = %q, want mention of new.txt", out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatalf("read new.txt: %v", err)
	}
	if string(got) != "hello\nworld\n" {
		t.Fatalf("new.txt = %q, want %q", got, "hello\nworld\n")
	}
}

func TestApplyPatchToolUpdateFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("line1\nold line\nline3\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	out, err := runApplyPatch(t, dir, "*** Begin Patch\n*** Update File: a.txt\n@@\n line1\n-old line\n+new line\n line3\n*** End Patch")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "a.txt") {
		t.Fatalf("output = %q, want mention of a.txt", out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != "line1\nnew line\nline3\n" {
		t.Fatalf("a.txt = %q, want %q", got, "line1\nnew line\nline3\n")
	}
}

func TestApplyPatchToolAtEOF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	out, err := runApplyPatch(t, dir, "*** Begin Patch\n*** Update File: a.txt\n@@\n-line2\n+line2b\n*** End of File\n*** End Patch")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "a.txt") {
		t.Fatalf("output = %q, want mention of a.txt", out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != "line1\nline2b\n" {
		t.Fatalf("a.txt = %q, want %q", got, "line1\nline2b\n")
	}
}

func TestApplyPatchToolDeleteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gone.txt")
	if err := os.WriteFile(path, []byte("bye\n"), 0o644); err != nil {
		t.Fatalf("write gone.txt: %v", err)
	}
	out, err := runApplyPatch(t, dir, "*** Begin Patch\n*** Delete File: gone.txt\n*** End Patch")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "gone.txt") {
		t.Fatalf("output = %q, want mention of gone.txt", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("gone.txt should be deleted, stat err = %v", err)
	}
}

// TestApplyPatchToolCRLFPreserved verifies updating a CRLF file keeps CRLF
// line endings in both the untouched and the replaced regions.
func TestApplyPatchToolCRLFPreserved(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "win.txt"), []byte("line1\r\nold line\r\nline3\r\n"), 0o644); err != nil {
		t.Fatalf("write win.txt: %v", err)
	}
	_, err := runApplyPatch(t, dir, "*** Begin Patch\n*** Update File: win.txt\n@@\n line1\n-old line\n+new line\n line3\n*** End Patch")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "win.txt"))
	if string(got) != "line1\r\nnew line\r\nline3\r\n" {
		t.Fatalf("win.txt = %q, want CRLF preserved", got)
	}
}

// TestApplyPatchToolAtomic verifies a failure in a later hunk leaves earlier
// files untouched (all-or-nothing).
func TestApplyPatchToolAtomic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	patch := "*** Begin Patch\n" +
		"*** Update File: a.txt\n@@\n-keep me\n+changed\n" +
		"*** Update File: b.txt\n@@\n-missing line\n+new\n" +
		"*** End Patch"
	_, err := runApplyPatch(t, dir, patch)
	if err == nil || !strings.Contains(err.Error(), "b.txt") {
		t.Fatalf("err = %v, want not-found error for b.txt", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != "keep me\n" {
		t.Fatalf("a.txt = %q, want untouched by failed patch (atomicity)", got)
	}
}

func TestApplyPatchToolNotUnique(t *testing.T) {
	dir := t.TempDir()
	content := "dup\nx\ndup\ny\n"
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	_, err := runApplyPatch(t, dir, "*** Begin Patch\n*** Update File: a.txt\n@@\n-dup\n+changed\n*** End Patch")
	if err == nil || !strings.Contains(err.Error(), "not unique") {
		t.Fatalf("err = %v, want not-unique error (no @@ anchor to disambiguate)", err)
	}
}

// TestApplyPatchToolAnchorDisambiguates verifies an "@@ <anchor>" line lets a
// non-unique block be placed correctly instead of failing the uniqueness
// check.
func TestApplyPatchToolAnchorDisambiguates(t *testing.T) {
	dir := t.TempDir()
	content := "func a() {\n    x()\n}\n\nfunc b() {\n    x()\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	patch := "*** Begin Patch\n*** Update File: a.txt\n@@ func b()\n func b() {\n-    x()\n+    y()\n }\n*** End Patch"
	_, err := runApplyPatch(t, dir, patch)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	want := "func a() {\n    x()\n}\n\nfunc b() {\n    y()\n}\n"
	if string(got) != want {
		t.Fatalf("a.txt = %q, want %q", got, want)
	}
}

func TestApplyPatchToolNotFound(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("line1\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	_, err := runApplyPatch(t, dir, "*** Begin Patch\n*** Update File: a.txt\n@@\n-absent line\n+new\n*** End Patch")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not-found error", err)
	}
}

func TestApplyPatchToolPermissionPreserved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not preserved on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("write run.sh: %v", err)
	}
	_, err := runApplyPatch(t, dir, "*** Begin Patch\n*** Update File: run.sh\n@@\n-echo hi\n+echo bye\n*** End Patch")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755 preserved", fi.Mode().Perm())
	}
}

func TestApplyPatchToolInvalidPatch(t *testing.T) {
	dir := t.TempDir()
	if _, err := runApplyPatch(t, dir, "not a patch"); err == nil {
		t.Fatal("invalid patch should fail")
	}
	if _, err := runApplyPatch(t, dir, ""); err == nil {
		t.Fatal("empty patch should fail")
	}
}
