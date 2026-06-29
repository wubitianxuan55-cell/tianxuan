package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrecheckMultiEdit_AllFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "test.go"))
	content := "line one\nline two\nline three\n"
	os.WriteFile(path, []byte(content), 0644)

	a := &AgentRunner{}
	msg := a.precheckMultiEdit([]byte(`{
		"path": "` + path + `",
		"edits": [
			{"old_string": "line one", "new_string": "LINE ONE"},
			{"old_string": "line three", "new_string": "LINE THREE"}
		]
	}`))
	if msg != "" {
		t.Fatalf("all old_strings present, should pass: %s", msg)
	}
}

func TestPrecheckMultiEdit_OneNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "test.go"))
	content := "line one\nline two\nline three\n"
	os.WriteFile(path, []byte(content), 0644)

	a := &AgentRunner{}
	msg := a.precheckMultiEdit([]byte(`{
		"path": "` + path + `",
		"edits": [
			{"old_string": "line one", "new_string": "x"},
			{"old_string": "LINE FOUR", "new_string": "y"},
			{"old_string": "line three", "new_string": "z"}
		]
	}`))
	if msg == "" {
		t.Fatal("LINE FOUR not in file, should block")
	}
	if !strings.Contains(msg, "precheck blocked") {
		t.Fatalf("msg should say precheck blocked: %s", msg)
	}
	if !strings.Contains(msg, "multi_edit[1]") {
		t.Fatalf("msg should mention which edit failed (index 1): %s", msg)
	}
}

func TestPrecheckMultiEdit_EmptyEdits(t *testing.T) {
	a := &AgentRunner{}
	msg := a.precheckMultiEdit([]byte(`{"path":"/tmp/x.go","edits":[]}`))
	if msg != "" {
		t.Fatal("empty edits should be silent")
	}
}

func TestPrecheckMultiEdit_MissingFile(t *testing.T) {
	a := &AgentRunner{}
	msg := a.precheckMultiEdit([]byte(`{
		"path": "/nonexistent/file.go",
		"edits": [{"old_string": "x", "new_string": "y"}]
	}`))
	if msg != "" {
		t.Fatalf("missing file should be silent: %s", msg)
	}
}

func TestPrecheckMultiEdit_EmptyOldString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "test.go"))
	os.WriteFile(path, []byte("content"), 0644)

	a := &AgentRunner{}
	// Empty old_string means "insert at position" — should not block.
	msg := a.precheckMultiEdit([]byte(`{
		"path": "` + path + `",
		"edits": [{"old_string": "", "new_string": "inserted"}]
	}`))
	if msg != "" {
		t.Fatalf("empty old_string should not block: %s", msg)
	}
}

func TestPrecheckDeleteRange_BothAnchorsFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "test.go"))
	content := "// start block\ncode here\nmore code\n// end block\n"
	os.WriteFile(path, []byte(content), 0644)

	a := &AgentRunner{}
	msg := a.precheckDeleteRange([]byte(`{
		"path": "` + path + `",
		"start_anchor": "// start block",
		"end_anchor": "// end block"
	}`))
	if msg != "" {
		t.Fatalf("both anchors present, should pass: %s", msg)
	}
}

func TestPrecheckDeleteRange_StartAnchorMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "test.go"))
	content := "code here\n// end block\n"
	os.WriteFile(path, []byte(content), 0644)

	a := &AgentRunner{}
	msg := a.precheckDeleteRange([]byte(`{
		"path": "` + path + `",
		"start_anchor": "// start block",
		"end_anchor": "// end block"
	}`))
	if msg == "" {
		t.Fatal("start_anchor missing, should block")
	}
	if !strings.Contains(msg, "start_anchor") {
		t.Fatalf("msg should mention start_anchor: %s", msg)
	}
}

func TestPrecheckDeleteRange_EndAnchorMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "test.go"))
	content := "// start block\ncode here\n"
	os.WriteFile(path, []byte(content), 0644)

	a := &AgentRunner{}
	msg := a.precheckDeleteRange([]byte(`{
		"path": "` + path + `",
		"start_anchor": "// start block",
		"end_anchor": "// end block"
	}`))
	if msg == "" {
		t.Fatal("end_anchor missing, should block")
	}
	if !strings.Contains(msg, "end_anchor") {
		t.Fatalf("msg should mention end_anchor: %s", msg)
	}
}

func TestPrecheckDeleteRange_BothAnchorsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "test.go"))
	os.WriteFile(path, []byte("unrelated content"), 0644)

	a := &AgentRunner{}
	msg := a.precheckDeleteRange([]byte(`{
		"path": "` + path + `",
		"start_anchor": "// start block",
		"end_anchor": "// end block"
	}`))
	if msg == "" {
		t.Fatal("both anchors missing, should block")
	}
	if !strings.Contains(msg, "start_anchor") || !strings.Contains(msg, "end_anchor") {
		t.Fatalf("msg should mention both anchors: %s", msg)
	}
}

func TestPrecheckDeleteRange_EmptyAnchors(t *testing.T) {
	a := &AgentRunner{}
	if msg := a.precheckDeleteRange([]byte(`{"path":"/tmp/x.go","start_anchor":"","end_anchor":"end"}`)); msg != "" {
		t.Fatal("empty start_anchor should be silent")
	}
	if msg := a.precheckDeleteRange([]byte(`{"path":"/tmp/x.go","start_anchor":"start","end_anchor":""}`)); msg != "" {
		t.Fatal("empty end_anchor should be silent")
	}
}

func TestPrecheckDeleteRange_MissingFile(t *testing.T) {
	a := &AgentRunner{}
	msg := a.precheckDeleteRange([]byte(`{
		"path": "/nonexistent/file.go",
		"start_anchor": "x",
		"end_anchor": "y"
	}`))
	if msg != "" {
		t.Fatalf("missing file should be silent: %s", msg)
	}
}

func TestPrecheckToolDispatch_MultiEditAndDeleteRange(t *testing.T) {
	a := &AgentRunner{}
	// These don't touch real files — they should just route to the right checker.
	if msg := a.precheckTool("multi_edit", []byte(`{"path":"/tmp/x","edits":[]}`)); msg != "" {
		t.Fatal("multi_edit with empty edits should be silent")
	}
	if msg := a.precheckTool("delete_range", []byte(`{"path":"/tmp/x","start_anchor":"","end_anchor":""}`)); msg != "" {
		t.Fatal("delete_range with empty anchors should be silent")
	}
}
