package boot

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"tianxuan/internal/cache"
	"tianxuan/internal/config"
	"tianxuan/internal/memory"
	"tianxuan/internal/outputstyle"
	"tianxuan/internal/skill"
	"tianxuan/internal/tool/builtin"
)

// syspromptOut contains the artifacts produced by building the system prompt.
type syspromptOut struct {
	prompt     string
	mem        *memory.Set
	skills     []skill.Skill
	compiler   *cache.Compiler
	runtimeCtx *cache.RuntimeLayer
	store      *skill.Store
}

// buildSystemPrompt assembles the L1 identity block: base system prompt +
// output style + language policy + persistent memory + skills index. It also
// scans the project profile and initialises the runtime context layer.
func buildSystemPrompt(cfg *config.Config, stderrPath io.Writer) (*syspromptOut, error) {
	sysPrompt, err := cfg.ResolveSystemPrompt()
	if err != nil {
		return nil, err
	}
	if st, ok := outputstyle.Resolve(cfg.Agent.OutputStyle, outputstyle.Dirs()); ok {
		sysPrompt = outputstyle.Apply(sysPrompt, st)
	}
	sysPrompt += "\n\n" + config.LanguagePolicy

	mem := memory.Load(memory.Options{CWD: ".", UserDir: config.MemoryUserDir()})
	sysPrompt = memory.Compose(sysPrompt, mem)
	builtin.SetMemorySearchIndex(mem.Search)
	builtin.SetSearchConfig(cfg.Search)
	if mem.Empty() {
		memory.InitDefaults(mem)
	}

	cwd, _ := os.Getwd()
	skillStore := skill.New(skill.Options{ProjectRoot: cwd, CustomPaths: cfg.SkillCustomPaths(), Stderr: stderrPath})
	skills := skillStore.List()
	sysPrompt = skill.ApplyIndex(sysPrompt, skills)

	// parallel dispatch guidance — tells the model WHEN to use parallel_tasks / parallel_skills
	sysPrompt += `
## Parallel dispatch

When you face 2+ independent tasks that can be worked on without shared state or
sequential dependencies, dispatch them in parallel instead of one-by-one:

- parallel_tasks — for arbitrary sub-agent tasks, read-only or with writes
  (e.g. "run tests for package A" + "run tests for package B",
   "build the frontend" + "build the backend",
   "find all callers of X in Go" + "find all consumers of X in frontend")
- parallel_skills — for named skill invocations (explore, review, research,
  security_review) that each need an isolated sub-agent
- bash run_in_background — for long-running commands (servers, watchers, builds)
  that you want to start now and check later with bash_output or wait

Decision: if two tasks don't share state and neither result depends on the other,
they are parallel-safe. When in doubt, default to parallel — sub-agents run in
isolated sessions. Multiple independent bash commands (e.g. tests for different
packages) should be dispatched via parallel_tasks, NOT chained serially in a
single turn.`

	builtin.WireReadSkillResolver(func(name string) (string, error) {
		sk, ok := skillStore.Read(name)
		if !ok {
			return "", fmt.Errorf("skill %q not found", name)
		}
		return sk.Body, nil
	})

	projectProfile := &cache.Profile{}
	projectProfile.Scan(cwd)
	compiler := cache.New(sysPrompt, nil)

	runtimeCtx := cache.NewRuntimeLayer()
	runtimeCtx.SetProject(cache.ProjectState{
		Language:     projectProfile.Language,
		Module:       projectProfile.Module,
		EntryPoints:  projectProfile.EntryPoints,
		TopDirs:      projectProfile.TopDirs,
		TotalFiles:   projectProfile.TotalFiles,
		Dependencies: projectProfile.Dependencies,
		RootPath:     filepath.Base(cwd),
	})
	runtimeCtx.SetCompactL2(true)

	return &syspromptOut{
		prompt:     sysPrompt,
		mem:        mem,
		skills:     skills,
		compiler:   compiler,
		runtimeCtx: runtimeCtx,
		store:      skillStore,
	}, nil
}
