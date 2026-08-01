package control

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseRefTokens(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"see @docs:doc://x and @src/main.go", []string{"docs:doc://x", "src/main.go"}},
		{"trailing @file.go.", []string{"file.go"}},
		{"dedup @a @a", []string{"a"}},
		{"no refs here", nil},
		{"email a@b.com keeps token", []string{"b.com"}},
	}
	for _, c := range cases {
		got := parseRefTokens(c.line)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseRefTokens(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestClassifyRef(t *testing.T) {
	known := map[string]bool{"docs": true}
	files := map[string]bool{"src/main.go": true, "README.md": true, ".tianxuan/attachments/clipboard-20260601-010203.000000.png": true}
	exists := func(p string) bool { return files[p] }

	cases := []struct {
		token   string
		wantOK  bool
		wantKnd refKind
	}{
		{"docs:doc://style", true, refResource}, // known server + uri
		{"src/main.go", true, refFile},          // existing file
		{"README.md", true, refFile},            // existing file
		{".tianxuan/attachments/clipboard-20260601-010203.000000.png", true, refImage},
		{"ghost:issue://1", false, 0}, // unknown server, no such file
		{"missing.go", false, 0},      // nonexistent path → not a ref
		{"docs:", false, 0},           // empty uri → not a resource, no file
	}
	for _, c := range cases {
		r, ok := classifyRef(c.token, known, exists)
		if ok != c.wantOK {
			t.Errorf("classifyRef(%q) ok = %v, want %v", c.token, ok, c.wantOK)
			continue
		}
		if ok && r.kind != c.wantKnd {
			t.Errorf("classifyRef(%q) kind = %v, want %v", c.token, r.kind, c.wantKnd)
		}
	}
}

// TestClassifyRefSession verifies @session:<id> resolves as a session ref
// without touching the filesystem.
func TestClassifyRefSession(t *testing.T) {
	known := map[string]bool{}
	exists := func(string) bool { return false }
	r, ok := classifyRef("session:abc123", known, exists)
	if !ok {
		t.Fatal("@session:<id> must classify as a session reference")
	}
	if r.kind != refSession {
		t.Fatalf("kind = %v, want refSession", r.kind)
	}
	if r.raw != "abc123" {
		t.Fatalf("raw session id = %q, want abc123", r.raw)
	}
}

// TestSessionDigestKeepsTextAndCompressesToolResults verifies the digest
// preserves user/assistant text, compresses tool results to one line, and
// keeps the newest content within budget.
func TestSessionDigestKeepsTextAndCompressesToolResults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	var b strings.Builder
	msgs := []struct {
		role, content string
	}{
		{"user", "修复登录超时"},
		{"assistant", "我来排查连接池配置"},
		{"tool", "读取 config.toml 成功，内容 2048 字节"},
		{"user", "结果如何"},
		{"assistant", "已修复，连接池大小调为 50"},
	}
	for _, m := range msgs {
		b.WriteString(`{"role":"` + m.role + `","content":"` + m.content + `"}` + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	digest, err := sessionDigest(path, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(digest, "修复登录超时") || !strings.Contains(digest, "已修复") {
		t.Fatalf("digest must keep user/assistant text:\n%s", digest)
	}
	if strings.Contains(digest, "2048 字节") {
		t.Fatalf("tool result must be compressed out of the digest:\n%s", digest)
	}
}

// TestSessionDigestBudgetKeepsNewest verifies a small budget drops older
// messages while keeping the newest tail and marks the omission.
func TestSessionDigestBudgetKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString(`{"role":"user","content":"message ` + string(rune('A'+i)) + `"}` + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := sessionDigest(path, 60) // tiny budget
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(digest, "omitted") && !strings.Contains(digest, "truncated") {
		t.Fatalf("small-budget digest must mark omitted earlier content:\n%s", digest)
	}
	if strings.Contains(digest, "message A") {
		t.Fatalf("small-budget digest must drop the oldest messages:\n%s", digest)
	}
	if !strings.Contains(digest, "message S") {
		t.Fatalf("small-budget digest must keep the newest tail:\n%s", digest)
	}
}

func TestReadFileRef(t *testing.T) {
	dir := t.TempDir()

	textPath := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(textPath, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(binPath, []byte{'a', 0x00, 'b'}, 0o644); err != nil {
		t.Fatal(err)
	}
	bigPath := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(bigPath, []byte(strings.Repeat("a", maxFileRefBytes+100)), 0o644); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(imagePath, []byte("\x89PNG\r\n\x1a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Text file: content verbatim, not a directory.
	if got, isDir, err := readFileRef(textPath); err != nil || isDir || got != "line one\nline two\n" {
		t.Errorf("text file = (%q, %v, %v)", got, isDir, err)
	}

	// Binary file: noted, not dumped.
	if got, _, err := readFileRef(binPath); err != nil || !strings.Contains(got, "binary file") {
		t.Errorf("binary file = (%q, %v), want a binary note", got, err)
	}

	// Image file: identified as image-specific guidance, not generic binary.
	if got, _, err := readFileRef(imagePath); err != nil || !strings.Contains(got, "image file") {
		t.Errorf("image file = (%q, %v), want an image note", got, err)
	}

	// Large file: truncated with a marker.
	if got, _, err := readFileRef(bigPath); err != nil || !strings.Contains(got, "truncated") {
		t.Errorf("big file should be truncated, got len=%d err=%v", len(got), err)
	}

	// Directory: recursive listing with relative paths including a trailing slash for subdirs.
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, isDir, err := readFileRef(dir)
	if err != nil || !isDir {
		t.Fatalf("dir = (isDir=%v, err=%v)", isDir, err)
	}
	if !strings.Contains(got, "hello.txt") || !strings.Contains(got, "sub/") || !strings.Contains(got, "sub/nested.txt") {
		t.Errorf("dir listing = %q, want hello.txt, sub/, and sub/nested.txt", got)
	}

	// Missing path: error.
	if _, _, err := readFileRef(filepath.Join(dir, "nope")); err == nil {
		t.Error("missing path should error")
	}
}
