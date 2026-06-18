package agent

import (
	"encoding/json"
	"testing"
)

func TestPlanBashCheckSafeCommands(t *testing.T) {
	safe := []string{
		"git status",
		"git diff HEAD~1",
		"git log --oneline -5",
		"ls -la",
		"grep -r \"func\" ./internal/",
		"find . -name '*.go'",
		"head -20 file.go",
		"go version",
		"go list ./...",
		"go vet ./internal/...",
		"go test -v -count=1 ./internal/agent/...",
		"go test ./... 2>&1",
		"python --version",
		"echo hello",
	}
	for _, cmd := range safe {
		args, _ := json.Marshal(map[string]string{"command": cmd})
		if reason := planBashCheck(args); reason != "" {
			t.Errorf("safe command %q was blocked: %s", cmd, reason)
		}
	}
}

func TestPlanBashCheckUnsafeCommands(t *testing.T) {
	unsafe := []string{
		"git status > file.txt",
		"ls > /tmp/x",
		"git pull",
		"git push",
		"npm install",
		"go build -o bin/app",
		"find . -name '*.go' -delete",
		"go fmt -fix ./...",
		"git status && git diff",
		"echo foo | grep bar",
		"curl https://example.com",
		"rm -rf /tmp/test",
	}
	for _, cmd := range unsafe {
		args, _ := json.Marshal(map[string]string{"command": cmd})
		if reason := planBashCheck(args); reason == "" {
			t.Errorf("unsafe command %q was allowed", cmd)
		}
	}
}

func TestHasShellRedirectAllowsStderr(t *testing.T) {
	if reason := hasShellRedirect("go test 2>&1"); reason != "" {
		t.Errorf("2>&1 should be allowed: %s", reason)
	}
	if reason := hasShellRedirect("go build -o x > /dev/null"); reason == "" {
		t.Error("file redirect should be blocked")
	}
	if reason := hasShellRedirect("echo \"> foo\""); reason != "" {
		t.Errorf("quoted > should be allowed: %s", reason)
	}
}
