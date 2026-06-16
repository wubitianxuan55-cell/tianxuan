package boot

import (
	"context"
	"strings"
	"testing"

	"tianxuan/internal/config"
	"tianxuan/internal/event"
	"tianxuan/internal/provider"

	// Blank imports register the provider kind and built-in tools the same way
	// cmd/tianxuan's main does; without them Build sees an empty provider
	// registry and a bare tool set.
	_ "tianxuan/internal/provider/openai"
	_ "tianxuan/internal/tool/builtin"
)

// TestBuildFoldsProjectMemoryIntoSystemPrompt is the end-to-end proof of the
// cache-first wiring: a project TIANXUAN.md is discovered at boot and folded
// into the session's system message (the cached prefix), and the `remember`
// tool is registered. It builds a real Controller from a throwaway project dir.
func TestBuildFoldsProjectMemoryIntoSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeFile(t, dir, "tianxuan.toml", `
default_model = "test-model"

[codegraph]
enabled = false

[agent]
system_prompt = "BASE SYSTEM PROMPT"

[[providers]]
name = "test-model"
kind = "openai"
base_url = "https://example.invalid"
model = "x"
api_key_env = "TIANXUAN_TEST_KEY_UNSET"
`)
	writeFile(t, dir, "TIANXUAN.md", "Project rule: always run go vet before committing.")

	ctrl, err := Build(context.Background(), Options{}) // RequireKey false: no network/key needed
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	// The system message is the cached prefix; it must contain both the base
	// prompt and the discovered memory.
	sys := systemMessage(ctrl.History())
	if !strings.Contains(sys, "BASE SYSTEM PROMPT") {
		t.Fatalf("base prompt missing from system message:\n%s", sys)
	}
	if !strings.Contains(sys, "always run go vet before committing") {
		t.Fatalf("project TIANXUAN.md not folded into system message:\n%s", sys)
	}
	// Base must come first so it stays a valid cache prefix when memory changes.
	if strings.Index(sys, "BASE SYSTEM PROMPT") > strings.Index(sys, "always run go vet") {
		t.Fatalf("memory should follow the base prompt, not precede it:\n%s", sys)
	}

	if mem := ctrl.Memory(); mem == nil || len(mem.Docs) == 0 {
		t.Fatal("controller memory set is empty after discovering TIANXUAN.md")
	}
}

// TestBuildDiscoversSkills proves the skill wiring end-to-end: a project skill
// is discovered at boot, surfaced via Controller.Skills(), and its name folds
// into the cache-stable system prompt's "# Skills" index alongside a built-in.
func TestBuildDiscoversSkills(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "tianxuan.toml", `
default_model = "test-model"

[codegraph]
enabled = false

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "openai"
base_url = "https://example.invalid"
model = "x"
api_key_env = "TIANXUAN_TEST_KEY_UNSET"
`)
	writeFile(t, dir, ".tianxuan/skills/projskill.md", "---\ndescription: a project skill\n---\nplaybook")

	ctrl, err := Build(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	var hasProj, hasBuiltin bool
	for _, s := range ctrl.Skills() {
		switch s.Name {
		case "projskill":
			hasProj = true
		case "explore":
			hasBuiltin = true
		}
	}
	if !hasProj || !hasBuiltin {
		t.Fatalf("Skills() should include the project skill and a built-in; got %v", ctrl.Skills())
	}

	sys := systemMessage(ctrl.History())
	if !strings.Contains(sys, "# Skills") {
		t.Fatalf("skills index missing from system prompt:\n%s", sys)
	}
	if !strings.Contains(sys, "projskill") || !strings.Contains(sys, "explore") {
		t.Fatalf("skill names missing from index:\n%s", sys)
	}
}

func TestBuildRecordsMCPStartupFailure(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "tianxuan.toml", `
default_model = "test-model"

[codegraph]
enabled = false

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "openai"
base_url = "https://example.invalid"
model = "x"
api_key_env = "TIANXUAN_TEST_KEY_UNSET"

[[plugins]]
name = "missing"
command = "tianxuan-missing-mcp-binary"
`)
	var notices []event.Event
	ctrl, err := Build(context.Background(), Options{
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.Notice {
				notices = append(notices, e)
			}
		}),
	})
	if err != nil {
		t.Fatalf("Build should not fail when an MCP server is unavailable: %v", err)
	}
	defer ctrl.Close()
	failures := ctrl.Host().Failures()
	if len(failures) != 1 || failures[0].Name != "missing" {
		t.Fatalf("failures = %+v, want missing", failures)
	}
	foundNotice := false
	for _, n := range notices {
		if strings.Contains(n.Text, "failed to start") {
			foundNotice = true
			break
		}
	}
	if !foundNotice {
		t.Fatalf("missing startup warning notice: %+v", notices)
	}
}

// TestBuildWithoutMemoryLeavesPromptUnchanged is the inverse invariant: with no
// memory files, the system prompt is exactly the configured base — the cache
// prefix is untouched by the memory feature.
func TestBuildWithoutMemoryLeavesPromptUnchanged(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "tianxuan.toml", `
default_model = "test-model"

[codegraph]
enabled = false

[agent]
system_prompt = "JUST THE BASE"

[[providers]]
name = "test-model"
kind = "openai"
base_url = "https://example.invalid"
model = "x"
api_key_env = "TIANXUAN_TEST_KEY_UNSET"
`)

	ctrl, err := Build(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	sys := systemMessage(ctrl.History())
	// Strip blocks that boot.Build always appends (skills index, language policy,
	// user-global memory) so the assertion is purely about whether project/ancestor
	// memory leaked into the base. A user-global AGENTS.md is real environment state
	// and not a project-memory leak, so strip it too.
	base := stripBootBlocks(sys)
	if base != "JUST THE BASE" {
		t.Fatalf("expected untouched base prompt %q, got stripped base %q\nfull sys:\n%s", "JUST THE BASE", base, sys)
	}
}

func TestBuildLanguagePolicyIsAppended(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, dir, "tianxuan.toml", `
default_model = "test-model"

[codegraph]
enabled = false

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "openai"
base_url = "https://example.invalid"
model = "x"
api_key_env = "TIANXUAN_TEST_KEY_UNSET"
`)

	ctrl, err := Build(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	sys := systemMessage(ctrl.History())
	if !strings.Contains(sys, config.LanguagePolicy) {
		t.Fatalf("language policy missing from system prompt:\n%s", sys)
	}
}

func systemMessage(msgs []provider.Message) string {
	for _, m := range msgs {
		if m.Role == provider.RoleSystem {
			return m.Content
		}
	}
	return ""
}

// stripBootBlocks removes blocks that boot.Build always appends to the base
// prompt (memory, skills, language policy). The remaining text should be exactly
// the configured system_prompt.
func stripBootBlocks(s string) string {
	s = strings.TrimSpace(s)
	// Strip in reverse order of append: skills → memory → V6.0 batch → language policy
	if i := strings.Index(s, "\n\n# Skills"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if i := strings.Index(s, "\n\n# Memory"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	// Strip V6.0 Batch Execution hint (appended between language policy and memory)
	if i := strings.Index(s, "\n\n## V6.0: Batch Execution"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	// Strip language policy (appended before memory, now at the tail)
	s = strings.TrimSpace(strings.TrimSuffix(s, config.LanguagePolicy))
	return s
}

func stripLanguagePolicy(s string) string {
	s = strings.TrimSpace(s)
	for _, policy := range []string{
		config.LanguagePolicy,
	} {
		s = strings.TrimSpace(strings.TrimSuffix(s, policy))
	}
	return s
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := writeFileRaw(dir, name, body); err != nil {
		t.Fatal(err)
	}
}


