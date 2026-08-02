package boot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBashPathEnvIncludesProjectTools locks the bash PATH augmentation: the
// project's bundled tools/go/bin (and node) must be injected when present, so
// the model never probes for go/node locations in every session.
func TestBashPathEnvIncludesProjectTools(t *testing.T) {
	root := t.TempDir()
	goBin := filepath.Join(root, "tools", "go", "bin")
	if err := os.MkdirAll(goBin, 0o755); err != nil {
		t.Fatal(err)
	}
	env := bashPathEnv(root)
	if len(env) == 0 {
		t.Fatal("bashPathEnv must return an entry when tools/go/bin exists")
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "PATH=") {
		t.Fatalf("env should carry a PATH entry, got %v", env)
	}
	if !strings.Contains(joined, goBin) {
		t.Fatalf("env should include the project go bin %q, got %v", goBin, env)
	}
}

// TestBashPathEnvExcludesMissingProjectTools: a project dir without bundled
// tools must not contribute its (nonexistent) tools paths to the env.
func TestBashPathEnvExcludesMissingProjectTools(t *testing.T) {
	root := t.TempDir() // no tools/ subdirs
	env := bashPathEnv(root)
	for _, kv := range env {
		if strings.Contains(kv, root) {
			t.Fatalf("env should not include paths under %s, got %q", root, kv)
		}
	}
}
