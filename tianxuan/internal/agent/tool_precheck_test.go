package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrecheckEditFileOldStringFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "test.go"))
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	os.WriteFile(path, []byte(content), 0644)

	a := &AgentRunner{}
	msg := a.precheckEditFile([]byte(`{"path":"` + path + `","old_string":"func main()","new_string":"func run()"}`))
	if msg != "" {
		t.Fatalf("old_string found, should pass: %s", msg)
	}
}

func TestPrecheckEditFileOldStringNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "test.go"))
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	os.WriteFile(path, []byte(content), 0644)

	a := &AgentRunner{}
	msg := a.precheckEditFile([]byte(`{"path":"` + path + `","old_string":"func missing()","new_string":"func run()"}`))
	if msg == "" {
		t.Fatal("old_string not in file, should block")
	}
	if !strings.Contains(msg, "precheck blocked") {
		t.Fatalf("msg should say precheck blocked: %s", msg)
	}
}

func TestPrecheckEditFileMissing(t *testing.T) {
	a := &AgentRunner{}
	msg := a.precheckEditFile([]byte(`{"path":"/nonexistent/file.go","old_string":"x","new_string":"y"}`))
	if msg != "" {
		t.Fatalf("missing file should be silent (let real Execute handle): %s", msg)
	}
}

func TestPrecheckEditFileEmptyArgs(t *testing.T) {
	a := &AgentRunner{}
	if msg := a.precheckEditFile([]byte(`{}`)); msg != "" {
		t.Fatal("empty args should be silent")
	}
	if msg := a.precheckEditFile([]byte(`{"path":"","old_string":"x"}`)); msg != "" {
		t.Fatal("empty path should be silent")
	}
	if msg := a.precheckEditFile([]byte(`{"path":"/tmp/x","old_string":""}`)); msg != "" {
		t.Fatal("empty old_string should be silent")
	}
}

func TestPrecheckToolDispatch(t *testing.T) {
	a := &AgentRunner{}
	if msg := a.precheckTool("unknown_tool", nil); msg != "" {
		t.Fatal("unknown tool should be silent")
	}
	if msg := a.precheckTool("read_file", []byte(`{"path":"/tmp/x"}`)); msg != "" {
		t.Fatal("read_file should be silent")
	}
	a.precheckTool("edit_file", []byte(`{"path":"/tmp/x","old_string":"y","new_string":"z"}`))
}
