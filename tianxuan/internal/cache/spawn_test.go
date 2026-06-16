package cache

import (
	"testing"
)

func TestRegisterAndLookupSpawnTemplate(t *testing.T) {
	// 确保测试间隔离
	spawnTemplates.items = nil

	st := SpawnTemplate{Kind: "test_kind", Prefix: "test prefix", Description: "test"}
	RegisterSpawnTemplate(st)

	got, ok := LookupSpawnTemplate("test_kind")
	if !ok {
		t.Fatal("LookupSpawnTemplate should find registered template")
	}
	if got.Prefix != "test prefix" {
		t.Fatalf("wrong prefix: got %q, want %q", got.Prefix, "test prefix")
	}
	if got.Description != "test" {
		t.Fatalf("wrong description: got %q, want %q", got.Description, "test")
	}

	// 未注册的 kind
	_, ok = LookupSpawnTemplate("nonexistent")
	if ok {
		t.Fatal("LookupSpawnTemplate should not find unregistered kind")
	}
}

func TestRegisterSpawnTemplate_EmptyKindOrPrefix(t *testing.T) {
	spawnTemplates.items = nil
	RegisterSpawnTemplate(SpawnTemplate{Kind: "", Prefix: "x", Description: ""})
	RegisterSpawnTemplate(SpawnTemplate{Kind: "k", Prefix: "", Description: ""})
	if len(spawnTemplates.items) != 0 {
		t.Fatal("should not register empty kind or prefix")
	}
}

func TestBuiltinSpawnTemplates(t *testing.T) {
	templates := BuiltinSpawnTemplates()
	if len(templates) != 4 {
		t.Fatalf("expected 4 builtin templates, got %d", len(templates))
	}
	types := make(map[string]bool)
	for _, st := range templates {
		if st.Prefix == "" {
			t.Fatalf("template %q has empty prefix", st.Kind)
		}
		if types[string(st.Kind)] {
			t.Fatalf("duplicate template kind: %s", st.Kind)
		}
		types[string(st.Kind)] = true
	}
}

func TestBuildSpawnPrompt_WithTemplate(t *testing.T) {
	spawnTemplates.items = nil
	RegisterSpawnTemplate(SpawnTemplate{Kind: "explore", Prefix: "explore prefix", Description: ""})
	p := NewSpawnPolicy()
	// mock ForkConfig
	cfg := ForkConfig{Task: "find main.go", TaskKind: "explore", Mode: ForkDefault}
	sysMsgs, userMsg := p.BuildSpawnPrompt("L1 system", cfg)
	if len(sysMsgs) != 2 {
		t.Fatalf("expected 2 system messages, got %d", len(sysMsgs))
	}
	if sysMsgs[0] != "L1 system" {
		t.Fatalf("sysMsgs[0] should be L1 system, got %q", sysMsgs[0])
	}
	if sysMsgs[1] != "explore prefix" {
		t.Fatalf("sysMsgs[1] should be template prefix, got %q", sysMsgs[1])
	}
	if userMsg != "find main.go" {
		t.Fatalf("userMsg should be task, got %q", userMsg)
	}
}

func TestBuildSpawnPrompt_NoTemplate(t *testing.T) {
	spawnTemplates.items = nil
	p := NewSpawnPolicy()
	cfg := ForkConfig{Task: "some task", TaskKind: "unknown", Mode: ForkDefault}
	sysMsgs, userMsg := p.BuildSpawnPrompt("L1 system", cfg)
	if len(sysMsgs) != 1 {
		t.Fatalf("expected 1 system message, got %d", len(sysMsgs))
	}
	if sysMsgs[0] != "L1 system" {
		t.Fatalf("sysMsgs[0] should be L1 system, got %q", sysMsgs[0])
	}
	if userMsg == "" || userMsg == "some task" {
		t.Fatalf("userMsg should contain merged prompt, got %q", userMsg)
	}
}
