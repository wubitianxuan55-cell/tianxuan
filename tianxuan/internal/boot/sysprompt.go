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

// GitWorkflowHint 是 L1 系统提示中的静态工作流规则(缓存安全:纯静态文本,
// 不注入实时 git 状态):多任务/长会话开始前主动检查分支并建议建功能分支,
// 任务完成按逻辑单元拆分提交,避免变更堆积在受保护分支。
const GitWorkflowHint = `## Git 工作流约定

- 多任务或长会话开始前,先用 git_status 检查工作区:若当前分支不是功能分支(如 master/main)且有未提交变更,建议用 git_worktree 创建功能分支再动手,避免变更堆积无法按任务提交。
- 任务完成时按逻辑单元拆分提交(git_diff 核对范围),不要一次堆积大量无关变更。
- 若提交被分支保护拒绝,先把变更迁到功能分支(git_worktree),不要绕过保护。`

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
	sysPrompt += "\n\n" + GitWorkflowHint
	sysPrompt += "\n\n" + config.LanguagePolicy

	mem := memory.Load(memory.Options{CWD: ".", UserDir: config.MemoryUserDir()})
	sysPrompt = memory.Compose(sysPrompt, mem)
	builtin.SetMemorySearchIndex(mem.Search)
	builtin.SetSearchConfig(cfg.Search)
	if mem.Empty() {
		memory.InitDefaults(mem)
	}

	cwd, _ := os.Getwd()

	// Ensure bundled skills are extracted to the global skills directory.
	// No-op after first run; a user's existing customisations are never
	// overwritten (EnsureBundled skips files that already exist).
	skill.EnsureBundled("")

	skillStore := skill.New(skill.Options{ProjectRoot: cwd, CustomPaths: cfg.SkillCustomPaths(), Stderr: stderrPath})
	skills := skillStore.List()
	// 已注册为独立工具的子代理技能（explore/research/review/security-review 及
	// 用户技能 runas: subagent）不再进入索引——工具 schema 已包含名称、描述与
	// 用法，索引重复列出只会增加前缀 token。inline 技能保留（模型仍需通过
	// 索引 + run_skill 发现它们）。
	var indexed []skill.Skill
	for _, sk := range skills {
		if sk.RunAs == skill.RunSubagent {
			continue
		}
		indexed = append(indexed, sk)
	}
	sysPrompt = skill.ApplyIndex(sysPrompt, indexed)

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

	// V10.96: 渐进式上下文阶梯 — 蒸馏自 SDL-MCP Iris Gate Ladder。
	// V10.99: 7 级实现梯子 — 蒸馏自 ponytail (89k⭐)。ponytail benchmark: −54% LOC −22% tokens。
	// 梯子在理解问题后爬，不是替代理解。原生优先于依赖，删除优于添加，无聊优于聪明。
	runtimeCtx.SetPromptHint("代码阶梯: lsp→grep→read_file(offset)→全文件。实现梯子(爬梯): (1)需要存在?→(2)已有?→(3)标准库?→(4)原生API?→(5)已装依赖?→(6)一行搞定?→(7)最少代码。原生优于依赖,删除优于添加,无聊优于聪明。")

	return &syspromptOut{
		prompt:     sysPrompt,
		mem:        mem,
		skills:     skills,
		compiler:   compiler,
		runtimeCtx: runtimeCtx,
		store:      skillStore,
	}, nil
}
