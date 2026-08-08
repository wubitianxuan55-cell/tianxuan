package agent

import (
	"context"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"

	_ "tianxuan/internal/tool/builtin" // register built-ins so LookupBuiltin works
)

// TestValidationErrorIncludesExample verifies validation errors carry a compact
// expected-args example (built from the tool schema) so the model can fix the
// call in one shot instead of re-reading the full schema.
func TestValidationErrorIncludesExample(t *testing.T) {
	bash, ok := tool.LookupBuiltin("bash")
	if !ok {
		t.Fatal("bash builtin not registered")
	}
	reg := tool.NewRegistry()
	reg.Add(bash)
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)

	out := a.executeOne(context.Background(), provider.ToolCall{Name: "bash", Arguments: `{}`})
	if !strings.Contains(out.output, "expected args like") {
		t.Fatalf("validation error should carry an example, got %q", out.output)
	}
	if !strings.Contains(out.output, "command") {
		t.Fatalf("example should name the command field, got %q", out.output)
	}
}

// TestMisuseHintSurfacesChatArgsOnBash verifies the known chat-API style
// argument mix-up on bash surfaces a corrective hint alongside the validation
// error, breaking the error loop at the source.
func TestMisuseHintSurfacesChatArgsOnBash(t *testing.T) {
	bash, ok := tool.LookupBuiltin("bash")
	if !ok {
		t.Fatal("bash builtin not registered")
	}
	reg := tool.NewRegistry()
	reg.Add(bash)
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)

	out := a.executeOne(context.Background(), provider.ToolCall{Name: "bash", Arguments: `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}`})
	if !strings.Contains(out.output, "chat-API") {
		t.Fatalf("chat args on bash should surface a misuse hint, got %q", out.output)
	}
	if !strings.Contains(out.output, "command") {
		t.Fatalf("the hint should name the command field, got %q", out.output)
	}
}
