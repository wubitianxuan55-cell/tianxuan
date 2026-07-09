package agent

import "testing"

func TestStripTransientBlocks_NoBlocks(t *testing.T) {
	input := "帮我修复bug"
	got := StripTransientBlocks(input)
	if got != input {
		t.Errorf("expected %q, got %q", input, got)
	}
}

func TestStripTransientBlocks_ReasoningOnly(t *testing.T) {
	input := `<reasoning-language>
use Simplified Chinese
</reasoning-language>

帮我修复bug`
	want := "帮我修复bug"
	got := StripTransientBlocks(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripTransientBlocks_ResponseOnly(t *testing.T) {
	input := `<response-language>
use Simplified Chinese
</response-language>

帮我修复bug`
	want := "帮我修复bug"
	got := StripTransientBlocks(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripTransientBlocks_BothBlocks(t *testing.T) {
	input := `<reasoning-language>
use Simplified Chinese
</reasoning-language>

<response-language>
use Simplified Chinese
</response-language>

帮我修复bug`
	want := "帮我修复bug"
	got := StripTransientBlocks(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripTransientBlocks_MemoryUpdate(t *testing.T) {
	input := `<memory-update>
added some facts
</memory-update>

请问`
	want := "请问"
	got := StripTransientBlocks(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripTransientBlocks_BackgroundJobs(t *testing.T) {
	input := `<background-jobs>
job 1 running
</background-jobs>

list files`
	want := "list files"
	got := StripTransientBlocks(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripTransientBlocks_AllBlocksChain(t *testing.T) {
	input := `<reasoning-language>zh</reasoning-language>
<response-language>zh</response-language>
<memory-update>facts</memory-update>
<session-facts>tmp</session-facts>
<background-jobs>jobs</background-jobs>
<procedural-rules>always do X</procedural-rules>
<episodic-memory>past experience</episodic-memory>
真正的用户输入`
	want := "真正的用户输入"
	got := StripTransientBlocks(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripTransientBlocks_SessionFacts(t *testing.T) {
	input := `<session-facts>
temp fact about project
</session-facts>

继续工作`
	want := "继续工作"
	got := StripTransientBlocks(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripTransientBlocks_ProceduralRules(t *testing.T) {
	input := `<procedural-rules>
These rules ALWAYS apply:
1. Never use panic()
</procedural-rules>

帮我写代码`
	want := "帮我写代码"
	got := StripTransientBlocks(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripTransientBlocks_EpisodicMemory(t *testing.T) {
	input := `<episodic-memory>
## Past fix for auth bug
Fixed by using bcrypt instead of sha256
</episodic-memory>

修复认证问题`
	want := "修复认证问题"
	got := StripTransientBlocks(input)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestStripTransientBlocks_Unclosed(t *testing.T) {
	input := `<reasoning-language>
unclosed block
`
	got := StripTransientBlocks(input)
	if got != input {
		t.Errorf("unclosed block should be returned as-is, got %q", got)
	}
}

func TestStripTransientBlocks_OnlyWhitespace(t *testing.T) {
	got := StripTransientBlocks("   \n\t  ")
	if got != "" {
		t.Errorf("whitespace-only input should return empty string, got %q", got)
	}
}

func TestStripTransientBlocks_Empty(t *testing.T) {
	got := StripTransientBlocks("")
	if got != "" {
		t.Errorf("empty input should return empty string, got %q", got)
	}
}
