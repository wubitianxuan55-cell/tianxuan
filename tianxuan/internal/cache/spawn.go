package cache

import (
	"crypto/sha256"
	"encoding/hex"
	
	"strings"
	"sync"
	"time"
)

type ForkMode int

const (
	ForkDefault ForkMode = iota
	ForkLight
	ForkWarm
)

// SpawnTemplate 定义子代理的 L4 固定模板——同类子代理共享相同的前缀字节，
// 使 DeepSeek 服务端前缀缓存可以命中该段。
type SpawnTemplate struct {
	Kind        TaskKind
	Prefix      string
	Description string
}

// spawnTemplates 是全局子代理模板注册表
var spawnTemplates struct {
	mu    sync.Mutex
	items map[TaskKind]SpawnTemplate
}

// RegisterSpawnTemplate 注册子代理模板。已注册模板的 Prefix 会被注入为子代理的
// 独立 System 消息（在 L1+L2 之后、User 任务描述之前），使得 DeepSeek 前缀缓存
// 可覆盖 [L1+L2+template] 这段字节。同类子代理共享模板→缓存命中。
func RegisterSpawnTemplate(st SpawnTemplate) {
	if st.Kind == "" || st.Prefix == "" {
		return
	}
	spawnTemplates.mu.Lock()
	defer spawnTemplates.mu.Unlock()
	if spawnTemplates.items == nil {
		spawnTemplates.items = make(map[TaskKind]SpawnTemplate)
	}
	spawnTemplates.items[st.Kind] = st
}

// LookupSpawnTemplate 查找已注册的子代理模板。
func LookupSpawnTemplate(kind TaskKind) (SpawnTemplate, bool) {
	spawnTemplates.mu.Lock()
	defer spawnTemplates.mu.Unlock()
	if spawnTemplates.items == nil {
		return SpawnTemplate{}, false
	}
	t, ok := spawnTemplates.items[kind]
	return t, ok
}

// ─── 内置子代理模板前缀 ─────────────────────────────────────
// 每个模板是固定文本 block，所有同类子代理使用完全相同文本→缓存命中。
// 每个精炼到 ≤200 tok，覆盖核心行为指令但不含具体任务内容。

const subagentExplorePrefix = `你是 tianxuan 的代码探索子代理。你的任务是在代码库中进行深入调查。

核心规则：
- 读取相关文件，理解代码结构和逻辑
- 只做只读操作，不修改任何文件
- 将调查结果以结构化摘要返回：关键发现、代码路径、调用链
- 返回应自包含——父代理只看到你的最终答案`

const subagentResearchPrefix = `你是 tianxuan 的外部调研子代理。你的任务是从外部资源收集信息。

核心规则：
- 使用 web_fetch 和 web_search 获取信息
- 引用来源，区分事实与推断
- 只做只读操作，不修改任何文件
- 返回结构化调研报告：结论、论据、来源`

const subagentReviewPrefix = `你是 tianxuan 的代码审查子代理。你的任务是审查代码变更。

核心规则：
- 读取变更文件，检查正确性、安全性、测试覆盖
- 只做只读操作，不修改任何文件
- 按严重程度分类问题：blocker / major / minor / nit
- 返回审查报告：问题列表 + 改进建议`

const subagentSecurityPrefix = `你是 tianxuan 的安全审计子代理。你的任务是审计代码的安全性。

核心规则：
- 检查：注入攻击、认证绕过、敏感信息泄露、权限提升
- 只做只读操作，不修改任何文件
- 按风险等级分类：critical / high / medium / low
- 返回审计报告：风险描述 + 攻击路径 + 修复建议`

// BuiltinSpawnTemplates 返回内置子代理模板列表，供 boot 阶段注册。
func BuiltinSpawnTemplates() []SpawnTemplate {
	return []SpawnTemplate{
		{Kind: TaskKind("subagent_explore"), Prefix: subagentExplorePrefix, Description: "代码探索"},
		{Kind: TaskKind("subagent_research"), Prefix: subagentResearchPrefix, Description: "外部调研"},
		{Kind: TaskKind("subagent_review"), Prefix: subagentReviewPrefix, Description: "代码审查"},
		{Kind: TaskKind("subagent_security"), Prefix: subagentSecurityPrefix, Description: "安全审计"},
	}
}

type SpawnPolicy struct {
	mu          sync.Mutex
	forkCount   int
	maxForks    int
	minTaskLen  int
	domainCache map[string]SpawnDomainEntry
	savedTokens int64
	savedUSD    float64
}

type SpawnDomainEntry struct {
	Hash      string
	Kind      TaskKind
	FirstUsed time.Time
	HitCount  int
}

type ForkConfig struct {
	Task       string
	TaskKind   TaskKind
	SkillNames []string
	Mode       ForkMode
}

type SpawnReport struct {
	ActiveForks int
	MaxForks    int
	SavedTokens int64
	SavedUSD    float64
	DomainCount int
	TotalSpawns int
}

func NewSpawnPolicy() *SpawnPolicy {
	return &SpawnPolicy{maxForks: 8, minTaskLen: 10, domainCache: make(map[string]SpawnDomainEntry, 64)}
}

func (p *SpawnPolicy) ShouldFork(task string) bool {
	trimmed := strings.TrimSpace(task)
	if len(trimmed) < p.minTaskLen { return false }
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.forkCount < p.maxForks
}

func (p *SpawnPolicy) Fork(compiler *Compiler, config ForkConfig) *Compiler {
	if compiler == nil { return nil }
	child := compiler.Fork()
	domain := p.buildSpawnDomain(config)
	p.mu.Lock()
	p.forkCount++
	p.recordHit(p.hashDomain(domain), config.TaskKind)
	p.mu.Unlock()
	return child
}

// BuildSpawnPrompt 构造子代理的完整 prompt。
//
// 新版：返回 (systemMessages, userMessage) 二元组。
// 当该 task kind 有注册模板时，模板作为独立的 system message 放在 L1 之后，
// 实际任务描述作为 user message——DeepSeek 可缓存 [L1+template] 这段前缀，
// 同类子代理（如所有 explore 调用）共享相同缓存。
//
// 无模板时（向后兼容）：整个合并到 user message 字符串返回。
func (p *SpawnPolicy) BuildSpawnPrompt(sysPrompt string, config ForkConfig) (systemMessages []string, userMessage string) {
	p.mu.Lock()
	p.forkCount++
	p.recordHit(p.hashDomain(p.buildSpawnDomain(config)), config.TaskKind)
	p.mu.Unlock()

	tmpl, hasTemplate := LookupSpawnTemplate(config.TaskKind)

	if hasTemplate && config.Mode == ForkDefault {
		msgs := make([]string, 0, 2)
		msgs = append(msgs, sysPrompt)   // msgs[0] = L1 (compiler.Fork() 继承)
		msgs = append(msgs, tmpl.Prefix) // msgs[1] = 模板（可缓存）
		return msgs, config.Task
	}

	// 向后兼容：无模板或非 default mode 时合并到 user message
	var b strings.Builder
	b.WriteString(sysPrompt)
	b.WriteString("\n\n--- spawn domain ---\n")
	b.WriteString(p.buildSpawnDomain(config))
	b.WriteString("\n---\n\n")
	b.WriteString(config.Task)
	return []string{sysPrompt}, b.String()
}

func (p *SpawnPolicy) buildSpawnDomain(config ForkConfig) string {
	var parts []string
	parts = append(parts, "## Spawn Domain")
	parts = append(parts, "- kind: "+string(config.TaskKind))
	switch config.Mode {
	case ForkLight: parts = append(parts, "- mode: light")
	case ForkWarm:  parts = append(parts, "- mode: warm")
	default:        parts = append(parts, "- mode: default")
	}
	if len(config.SkillNames) > 0 {
		parts = append(parts, "- skills: "+strings.Join(config.SkillNames, ", "))
	}
	return strings.Join(parts, "\n")
}

func (p *SpawnPolicy) hashDomain(domain string) string {
	h := sha256.Sum256([]byte(domain))
	return hex.EncodeToString(h[:])
}

func (p *SpawnPolicy) recordHit(hash string, kind TaskKind) {
	entry, exists := p.domainCache[hash]
	if exists {
		entry.HitCount++
		p.domainCache[hash] = entry
	} else {
		p.domainCache[hash] = SpawnDomainEntry{Hash: hash, Kind: kind, FirstUsed: time.Now(), HitCount: 1}
	}
	if len(p.domainCache) > 64 {
		var oldestKey string
		var oldestTime time.Time
		for k, e := range p.domainCache {
			if oldestKey == "" || e.FirstUsed.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.FirstUsed
			}
		}
		delete(p.domainCache, oldestKey)
	}
}

func (p *SpawnPolicy) RecordForkSavings(savedTokens int64, pricePerToken float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.savedTokens += savedTokens
	if pricePerToken > 0 { p.savedUSD += float64(savedTokens) * pricePerToken }
}

func (p *SpawnPolicy) ForkMetrics() (active, max int, savedTokens int64, savedUSD float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.forkCount, p.maxForks, p.savedTokens, p.savedUSD
}

func (p *SpawnPolicy) DomainReuseRate() (distinct, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	distinct = len(p.domainCache)
	for _, e := range p.domainCache { total += e.HitCount }
	return distinct, total
}

func (p *SpawnPolicy) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.forkCount = 0
	p.savedTokens = 0
	p.savedUSD = 0
	p.domainCache = make(map[string]SpawnDomainEntry, 64)
}

func (p *SpawnPolicy) Report() SpawnReport {
	active, max, savedTokens, savedUSD := p.ForkMetrics()
	distinct, total := p.DomainReuseRate()
	return SpawnReport{ActiveForks: active, MaxForks: max,
		SavedTokens: savedTokens, SavedUSD: savedUSD,
		DomainCount: distinct, TotalSpawns: total}
}
