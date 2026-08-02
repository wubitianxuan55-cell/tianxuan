package boot

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"tianxuan/internal/sandbox"
	"tianxuan/internal/tool"
)

// TestAddBuiltinsInjectsBashTimeout locks the config-to-tool wiring: the
// bash tool registered by addBuiltins actually applies the host-injected
// foreground cap (previously the config field was a dead field and the
// timeout was hard-coded).
func TestAddBuiltinsInjectsBashTimeout(t *testing.T) {
	reg := tool.NewRegistry()
	addBuiltins(reg, nil, nil, sandbox.Spec{}, 150*time.Millisecond, nil, io.Discard)

	bt, ok := reg.Get("bash")
	if !ok {
		t.Fatal("bash not registered")
	}
	sh := sandbox.ResolveShell()
	cmd := "sleep 2"
	if sh.Kind == sandbox.ShellPowerShell {
		cmd = "Start-Sleep -Seconds 2"
	}
	args, _ := json.Marshal(map[string]string{"command": cmd})

	start := time.Now()
	_, err := bt.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("bash timeout not injected: err=%v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("bash timeout fired too slowly: %v", elapsed)
	}
}
