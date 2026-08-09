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

// TestForkContextText verifies forkContextText extracts the last user input and
// the turns that follow (Qwen /fork semantics), ignoring anything before it.
func TestForkContextText(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "old unrelated request"},
		{Role: provider.RoleAssistant, Content: "old answer"},
		{Role: provider.RoleUser, Content: "fix the auth flow"},
		{Role: provider.RoleTool, Content: "auth.go: 12 lines"},
		{Role: provider.RoleAssistant, Content: "found the bug"},
	}
	got := forkContextText(msgs)
	if strings.Contains(got, "old unrelated request") || strings.Contains(got, "old answer") {
		t.Fatalf("fork context must start at the last user input, got:\n%s", got)
	}
	if !strings.Contains(got, "fix the auth flow") || !strings.Contains(got, "found the bug") {
		t.Fatalf("fork context must include the last user input and following turns, got:\n%s", got)
	}
}

// TestForkContextTextEmpty verifies an empty / system-only conversation yields
// no fork context instead of garbage.
func TestForkContextTextEmpty(t *testing.T) {
	if got := forkContextText(nil); got != "" {
		t.Fatalf("nil conversation should yield empty context, got %q", got)
	}
	if got := forkContextText([]provider.Message{
		{Role: provider.RoleSystem, Content: "sys"},
	}); got != "" {
		t.Fatalf("system-only conversation should yield empty context, got %q", got)
	}
}

// TestForkContextTextTruncates verifies the extraction honors the budget and
// never returns content larger than it plus a small slack.
func TestForkContextTextTruncates(t *testing.T) {
	long := strings.Repeat("x", 10_000)
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: long},
		{Role: provider.RoleAssistant, Content: long},
	}
	got := forkContextText(msgs)
	if len(got) > 5000 {
		t.Fatalf("fork context not truncated: %d bytes", len(got))
	}
	if !strings.Contains(got, "user: ") || !strings.Contains(got, "assistant: ") {
		t.Fatalf("roles missing from fork context: %q", got[:40])
	}
}

// captureProvider is a minimal Provider stub that records the Config it was
// built with, letting tests assert what boot forwards to provider factories.
type captureProvider struct{ cfg provider.Config }

func (p *captureProvider) Name() string { return p.cfg.Name }

func (p *captureProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk)
	close(ch)
	return ch, nil
}

// TestBuildProviderForwardsReasoningProtocol guards the config plumbing for the
// DeepSeek thinking round-trip: a user-set reasoning_protocol must reach the
// provider factory via Config.Extra (it previously silently died in boot, so
// Zen deepseek models 400'd with "reasoning_content ... must be passed back").
func TestBuildProviderForwardsReasoningProtocol(t *testing.T) {
	kind := "capture-reasoning-__" + t.Name()
	var got provider.Config
	provider.Register(kind, func(cfg provider.Config) (provider.Provider, error) {
		got = cfg
		return &captureProvider{cfg: cfg}, nil
	})

	p, err := buildProvider(&config.ProviderEntry{
		Kind:              kind,
		Name:              "zen",
		BaseURL:           "https://opencode.ai/zen/v1",
		Model:             "deepseek-v4-flash-free",
		ReasoningProtocol: "deepseek",
	}, "deepseek-v4-flash-free")
	if err != nil {
		t.Fatalf("buildProvider: %v", err)
	}
	if got.Name != "zen" || got.Model != "deepseek-v4-flash-free" {
		t.Fatalf("Config forwarded wrong identity: %+v", got)
	}
	if got.Extra["reasoning_protocol"] != "deepseek" {
		t.Errorf("Extra[reasoning_protocol] = %v, want \"deepseek\"", got.Extra["reasoning_protocol"])
	}
	if p.Name() != "zen" {
		t.Errorf("provider Name = %q, want zen", p.Name())
	}
}

// TestBuildFoldsProjectMemoryIntoSystemPrompt is the end-to-end proof of the
// cache-first wiring: a project TIANXUAN.md is discovered at boot and folded
// into the session's system message (the cached prefix), and the `remember`
// tool is registered. It builds a real Controller from a throwaway project dir.
func TestBuildFoldsProjectMemoryIntoSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir) // isolate from user-global config (~/.config/tianxuan/)
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
	t.Setenv("APPDATA", dir)
	t.Setenv("USERPROFILE", dir) // isolate the global skills dir too
	t.Setenv("HOME", dir)
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
	if !strings.Contains(sys, "projskill") {
		t.Fatalf("inline skill missing from index:\n%s", sys)
	}
	// Subagent skills are registered as dedicated tools (their schemas carry
	// name + description), so they must not be duplicated in the index.
	if i := strings.Index(sys, "# Skills"); i >= 0 {
		block := sys[i:]
		if strings.Contains(block, "- explore") || strings.Contains(block, "- banner-design") || strings.Contains(block, "- ui-ux-pro-max") {
			t.Fatalf("subagent skill should not be listed in the index (it is a dedicated tool):\n%s", block)
		}
	} else {
		t.Fatalf("subagent skill should not be listed in the index (it is a dedicated tool):\n%s", sys)
	}
}

// TestBuildRegistersSubagentSkillTools verifies every runAs=subagent skill is
// registered as a first-class, model-visible tool — the built-ins
// (explore/research/review/security_review) plus a user skill that opts in via
// frontmatter (banner-design) — rather than being hidden behind run_skill.
func TestBuildRegistersSubagentSkillTools(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("USERPROFILE", dir) // isolate the global skills dir too
	t.Setenv("HOME", dir)
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
	writeFile(t, dir, ".tianxuan/skills/banner-design/SKILL.md", "---\nname: banner-design\ndescription: Banner design\nrunas: subagent\nallowed-tools: read_file, write_file\n---\nbody")
	writeFile(t, dir, ".tianxuan/skills/taste-skill/SKILL.md", "---\nname: taste-skill\ndescription: Frontend aesthetic judgment\nrunas: subagent\nallowed-tools: read_file, ls, grep, bash\n---\nbody")

	ctrl, err := Build(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	names := ctrl.ToolNames()
	for _, want := range []string{"explore", "research", "review", "security_review", "banner_design", "taste_skill", "ui_styling", "design_router", "run_skill", "task"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tool %q not visible in executor registry; got %v", want, names)
		}
	}
}

func TestBuildRecordsMCPStartupFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
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
	t.Setenv("APPDATA", dir)
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
	t.Setenv("APPDATA", dir)
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

// TestBuildGitWorkflowHintPresent: 多任务开始前提示检查分支/建功能分支的
// 静态规则必须进入 L1 系统提示(缓存安全:静态文本,不注入实时 git 状态)。
func TestBuildGitWorkflowHintPresent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
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
	if !strings.Contains(sys, GitWorkflowHint) {
		t.Fatalf("git workflow hint missing from system prompt:\n%s", sys)
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
	// Strip git workflow hint (appended right before the language policy)
	if i := strings.Index(s, "\n\n## Git 工作流约定"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
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
