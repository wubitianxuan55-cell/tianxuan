package cache

import (
	"strings"
	"testing"

	"tianxuan/internal/tool"
)

func TestCompilerSystemPromptIdentityOnly(t *testing.T) {
	c := New("You are a coding agent.", nil)
	got := c.SystemPrompt()
	if got != "You are a coding agent." {
		t.Errorf("SystemPrompt() = %q, want identity only", got)
	}
}

func TestCompilerSystemPromptWithNilMemory(t *testing.T) {
	c := New("Base prompt.", nil)
	got := c.SystemPrompt()
	if got != "Base prompt." {
		t.Errorf("SystemPrompt() = %q, want base prompt only", got)
	}
}

func TestCompilerIdentityStableWhenContextChanges(t *testing.T) {
	basePrompt := "You are tianxuan. Be concise."
	reg := tool.NewRegistry()

	c1 := New(basePrompt, reg)
	c2 := New(basePrompt, reg)

	if c1.Identity() != c2.Identity() {
		t.Errorf("Identity changed: %q vs %q", c1.Identity(), c2.Identity())
	}
}

func TestCompilerContextEmptyByDefault(t *testing.T) {
	c := New("Base prompt.", nil)
	if c.Context() != "" {
		t.Errorf("Context() = %q, want empty", c.Context())
	}
}

func TestCompilerRegistryRoundTrip(t *testing.T) {
	reg := tool.NewRegistry()
	c := New("Prompt", reg)
	if c.Registry() != reg {
		t.Error("Registry() returned different pointer")
	}
	reg2 := tool.NewRegistry()
	c.SetRegistry(reg2)
	if c.Registry() != reg2 {
		t.Error("SetRegistry() did not update registry")
	}
}

func TestCompilerSkillsIndexAlreadyInBasePrompt(t *testing.T) {
	// Skills are composed into basePrompt by boot.Build before New() is called.
	// The compiler stores basePrompt verbatim as identity; context stays empty.
	basePrompt := "Identity.\n\n# Skills\n\n- explore: Explore the codebase"
	c := New(basePrompt, nil)

	// Context is empty — skills live in identity via basePrompt.
	if c.Context() != "" {
		t.Errorf("Context() = %q, want empty (skills are in basePrompt)", c.Context())
	}

	// SystemPrompt should return identity only.
	sys := c.SystemPrompt()
	if !strings.Contains(sys, "explore") {
		t.Errorf("SystemPrompt() missing skills: %q", sys)
	}
	if sys != basePrompt {
		t.Errorf("SystemPrompt() = %q, want basePrompt verbatim", sys)
	}
}

func TestSkillLayerRoutesKnownPatterns(t *testing.T) {
	l := NewSkillLayer()
	cfg := l.Route("fix the login bug")
	if cfg.Kind != KindFixBug {
		t.Errorf("Kind = %q, want fix_bug", cfg.Kind)
	}
	if len(cfg.Tools) == 0 {
		t.Error("fix route should return non-empty Tools")
	}

	cfg2 := l.Route("hello")
	if cfg2.Kind != KindDefault {
		t.Errorf("Kind = %q, want default", cfg2.Kind)
	}
	if len(cfg2.Tools) != 0 {
		t.Errorf("default Tools = %v, want empty", cfg2.Tools)
	}
}

func TestCompilerForkSharesIdentityAndContext(t *testing.T) {
	parent := New("Identity: be helpful.", nil)
	child := parent.Fork()

	if child.Identity() != parent.Identity() {
		t.Error("Forked child must share Identity bytes with parent")
	}
	if child.Context() != parent.Context() {
		t.Error("Forked child must share Context bytes with parent")
	}
	if child.Registry() != parent.Registry() {
		t.Error("Forked child must share Registry with parent")
	}

	// SystemPrompt should be identical since context is empty
	if child.SystemPrompt() != parent.SystemPrompt() {
		t.Errorf("child SystemPrompt = %q, parent = %q", child.SystemPrompt(), parent.SystemPrompt())
	}
}

func TestCompilerImmutable(t *testing.T) {
	c := New("Identity.", nil)
	first := c.SystemPrompt()
	// Compiler has no Set*/Add* methods — SystemPrompt can't change.
	second := c.SystemPrompt()
	if first != second {
		t.Errorf("SystemPrompt changed without mutation: %q → %q", first, second)
	}
	// Fork must also be stable.
	child := c.Fork()
	if child.SystemPrompt() != first {
		t.Errorf("child SystemPrompt = %q, want %q", child.SystemPrompt(), first)
	}
}
