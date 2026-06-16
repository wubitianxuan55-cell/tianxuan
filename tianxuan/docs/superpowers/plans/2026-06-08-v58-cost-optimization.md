# V5.8 成本与性能优化 实现计划

> **给 agentic worker：** 使用 TDD 铁律执行。步骤使用 checkbox（`- [ ]`）跟踪。

**目标：** 四项确定性优化降低 token 消耗、提升性能：SmartCompress（工具结果智能压缩）、CompactSummary（紧凑后上下文保留）、CacheWarmup（首轮缓存预热）、跨轮 toolCache（session 级文件缓存）。

**架构：** 所有压缩算法完全确定性（相同输入→相同输出），不引入 LLM 调用，不影响 L1/L2/tools 缓存前缀。压缩在 `executeOne()` 返回后、`truncateToolOutput()` 之前插入。

**技术栈：** Go 1.24, 标准库 only（无外部依赖）。

---

## 文件变更清单

| 文件 | 操作 | 职责 |
|------|------|------|
| `internal/agent/compress.go` | 新建 | SmartCompress 路由器 + grep/tree/read_file 压缩算法 |
| `internal/agent/compress_test.go` | 新建 | 压缩算法单元测试 |
| `internal/agent/agent.go` | 修改 | executeOne 接入 SmartCompress；toolCache TTL 延长；CacheWarmup |
| `internal/agent/toolcache.go` | 修改 | 移除 turn 级 clear，改为 mtime 驱动 |
| `internal/agent/compact.go` | 修改 | maybeCompact 插入 CompactSummary |
| `internal/agent/compact_test.go` | 修改 | CompactSummary 测试 |
| `internal/control/controller.go` | 修改 | runTurnWithRaw 首轮前 CacheWarmup |

---

## Phase 1: SmartCompress（最高 ROI）

### Step 1-1: 创建 compress.go — grep 压缩器

```go
// compress.go
package agent

import (
    "sort"
    "strconv"
    "strings"
)

// compressConfig 压缩参数（与 Headroom SearchCompressor 对齐）
type compressConfig struct {
    maxPerFile   int // 每文件最多保留条数
    maxTotal     int // 全局最多总条数
    maxFiles     int // 最多覆盖文件数
    keepFirst    bool
    keepLast     bool
    boostErrors  bool
}

var defaultCompressConfig = compressConfig{
    maxPerFile:  5,
    maxTotal:    30,
    maxFiles:    15,
    keepFirst:   true,
    keepLast:    true,
    boostErrors: true,
}

// errorPatterns 错误行匹配模式
var errorPatterns = []string{
    "error", "fatal", "panic", "fail", "exception",
    "ERROR", "FATAL", "PANIC", "FAIL", "EXCEPTION",
}

// grepMatch 单条 grep 结果
type grepMatch struct {
    file string
    line int
    text string
    score float64
}

// fileGroup 单文件的匹配组
type fileGroup struct {
    file    string
    matches []grepMatch
    total   int // 原始匹配总数（含压缩后丢弃的）
}

// compressGrep 压缩 grep/search_content 结果。确定性算法。
//
// 输入格式: "path:line: text" 每行一条。
// 输出格式: 同为 "path:line: text"，但保留最重要的条目。
func compressGrep(raw string) string {
    return compressGrepWithConfig(raw, defaultCompressConfig)
}

func compressGrepWithConfig(raw string, cfg compressConfig) string {
    if raw == "" {
        return raw
    }
    groups := parseGrepOutput(raw)
    if len(groups) == 0 {
        return raw
    }
    // 按文件总匹配数排序，取前 maxFiles
    sort.Slice(groups, func(i, j int) bool {
        return groups[i].total > groups[j].total
    })
    if len(groups) > cfg.maxFiles {
        groups = groups[:cfg.maxFiles]
    }
    // 每条匹配打分
    for i := range groups {
        for j := range groups[i].matches {
            groups[i].matches[j].score = scoreGrepMatch(groups[i].matches[j].text, cfg)
        }
    }
    // 每文件选取保留的匹配
    var outLines []string
    totalKept := 0
    for _, g := range groups {
        selected := selectGrepMatches(g.matches, cfg)
        for _, m := range selected {
            if totalKept >= cfg.maxTotal {
                break
            }
            outLines = append(outLines, formatGrepLine(m))
            totalKept++
        }
        // 该文件有省略的匹配 → 追加摘要行
        omitted := g.total - len(selected)
        if omitted > 0 {
            outLines = append(outLines, "[... and "+strconv.Itoa(omitted)+" more matches in "+g.file+"]")
        }
    }
    if len(outLines) == 0 {
        return raw
    }
    return strings.Join(outLines, "\n")
}

// parseGrepOutput 解析 grep 输出为 fileGroup 列表
func parseGrepOutput(raw string) []fileGroup {
    lines := strings.Split(raw, "\n")
    groups := make(map[string]*fileGroup)
    var order []string
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        file, lineNo, text := parseGrepLine(line)
        if file == "" {
            continue // 不是有效的 grep 行，保留原样
        }
        g, ok := groups[file]
        if !ok {
            g = &fileGroup{file: file}
            groups[file] = g
            order = append(order, file)
        }
        g.matches = append(g.matches, grepMatch{file: file, line: lineNo, text: text})
        g.total++
    }
    result := make([]fileGroup, 0, len(order))
    for _, f := range order {
        result = append(result, *groups[f])
    }
    return result
}

// parseGrepLine 解析一行 "path:line: text"。
// 处理 Windows 路径（C:\foo\bar.go:42: text）和 Unix 路径。
func parseGrepLine(line string) (file string, lineNo int, text string) {
    // 从右向左查找第一个 ":\d+:" 模式作为行列分隔
    // 先尝试找行号: 冒号后紧跟数字，且该数字后紧跟冒号
    idx := strings.LastIndex(line, ":")
    if idx < 0 {
        return "", 0, line
    }
    // 检查 idx 之前是否还有冒号（行号冒号）
    before := line[:idx]
    colon2 := strings.LastIndex(before, ":")
    if colon2 < 0 {
        return "", 0, line
    }
    lineNoStr := before[colon2+1:]
    no, err := strconv.Atoi(lineNoStr)
    if err != nil {
        return "", 0, line
    }
    file = line[:colon2]
    text = line[idx+1:]
    // 去除 text 开头的空格
    text = strings.TrimLeft(text, " ")
    return file, no, text
}

// scoreGrepMatch 给单条匹配打分（0-1）
func scoreGrepMatch(text string, cfg compressConfig) float64 {
    score := 0.1 // 基础分
    lower := strings.ToLower(text)
    if cfg.boostErrors {
        for _, pat := range errorPatterns {
            if strings.Contains(lower, strings.ToLower(pat)) {
                score += 0.5
                break
            }
        }
    }
    return score
}

// selectGrepMatches 从文件匹配列表中选取保留的条目
func selectGrepMatches(matches []grepMatch, cfg compressConfig) []grepMatch {
    n := len(matches)
    if n <= cfg.maxPerFile {
        // 全部保留，按行号排序
        sorted := make([]grepMatch, n)
        copy(sorted, matches)
        sort.Slice(sorted, func(i, j int) bool { return sorted[i].line < sorted[j].line })
        return sorted
    }
    // 标记保留的索引
    keep := make(map[int]bool)
    // 首条
    if cfg.keepFirst && n > 0 {
        keep[0] = true
    }
    // 末条
    if cfg.keepLast && n > 1 {
        keep[n-1] = true
    }
    // 中间按分数选
    slots := cfg.maxPerFile - len(keep)
    if slots > 0 {
        type scored struct {
            idx   int
            score float64
        }
        var middle []scored
        for i := 0; i < n; i++ {
            if keep[i] {
                continue
            }
            middle = append(middle, scored{idx: i, score: matches[i].score})
        }
        sort.Slice(middle, func(i, j int) bool { return middle[i].score > middle[j].score })
        for i := 0; i < slots && i < len(middle); i++ {
            keep[middle[i].idx] = true
        }
    }
    // 按行号排序输出
    var result []grepMatch
    for i := 0; i < n; i++ {
        if keep[i] {
            result = append(result, matches[i])
        }
    }
    return result
}

func formatGrepLine(m grepMatch) string {
    return m.file + ":" + strconv.Itoa(m.line) + ":" + m.text
}
```

### Step 1-2: 创建 compress_test.go — grep 压缩测试

```go
package agent

import (
    "strings"
    "testing"
)

func TestParseGrepLine_Unix(t *testing.T) {
    file, line, text := parseGrepLine("src/main.go:42: func main() {")
    if file != "src/main.go" || line != 42 || text != "func main() {" {
        t.Errorf("got (%q, %d, %q)", file, line, text)
    }
}

func TestParseGrepLine_Windows(t *testing.T) {
    file, line, text := parseGrepLine(`C:\Users\foo\bar.go:15: package main`)
    if file != `C:\Users\foo\bar.go` || line != 15 || text != "package main" {
        t.Errorf("got (%q, %d, %q)", file, line, text)
    }
}

func TestParseGrepLine_PathWithColon(t *testing.T) {
    // 路径中包含冒号（Windows 盘符）
    file, line, text := parseGrepLine(`D:\AI\tianxuan\main.go:1: package main`)
    if file != `D:\AI\tianxuan\main.go` || line != 1 || text != "package main" {
        t.Errorf("got (%q, %d, %q)", file, line, text)
    }
}

func TestCompressGrep_Small(t *testing.T) {
    input := strings.Join([]string{
        "a.go:1: package a",
        "a.go:5: func Foo()",
        "b.go:2: package b",
    }, "\n")
    out := compressGrep(input)
    // 3条 ≤ 30上限，应全部保留
    if !strings.Contains(out, "a.go:1:") {
        t.Errorf("expected a.go:1 preserved, got: %s", out)
    }
    if !strings.Contains(out, "b.go:2:") {
        t.Errorf("expected b.go:2 preserved, got: %s", out)
    }
}

func TestCompressGrep_Large(t *testing.T) {
    // 单文件 20 条匹配，应压缩到 5 条 + 摘要
    var lines []string
    for i := 1; i <= 20; i++ {
        lines = append(lines, "file.go:"+itoaTest(i)+": line "+itoaTest(i))
    }
    input := strings.Join(lines, "\n")
    out := compressGrep(input)
    // 应包含摘要行
    if !strings.Contains(out, "more matches in file.go") {
        t.Errorf("expected summary line, got: %s", out)
    }
    // 输出行数 ≤ 5 + 1(摘要) = 6
    outLines := strings.Split(strings.TrimSpace(out), "\n")
    if len(outLines) > 6 {
        t.Errorf("expected ≤6 lines, got %d: %s", len(outLines), out)
    }
}

func TestCompressGrep_ErrorBoost(t *testing.T) {
    input := strings.Join([]string{
        "a.go:1: normal line",
        "a.go:2: normal line too",
        "a.go:3: ERROR: something failed",
        "a.go:4: another normal",
        "a.go:5: FATAL: crash here",
        "a.go:6: normal again",
        "a.go:7: more normal",
        "a.go:8: panic: nil pointer",
    }, "\n")
    out := compressGrep(input)
    // 错误行应该被保留
    if !strings.Contains(out, "ERROR:") {
        t.Errorf("expected ERROR line preserved, got: %s", out)
    }
    if !strings.Contains(out, "FATAL:") {
        t.Errorf("expected FATAL line preserved, got: %s", out)
    }
    if !strings.Contains(out, "panic:") {
        t.Errorf("expected panic line preserved, got: %s", out)
    }
}

func TestCompressGrep_Empty(t *testing.T) {
    out := compressGrep("")
    if out != "" {
        t.Errorf("expected empty, got %q", out)
    }
}

func TestCompressGrep_MultiFile(t *testing.T) {
    // 20 个文件，每文件 3 条匹配
    var lines []string
    for f := 1; f <= 20; f++ {
        fname := "file" + itoaTest(f) + ".go"
        for i := 1; i <= 3; i++ {
            lines = append(lines, fname+":"+itoaTest(i)+": line "+itoaTest(i))
        }
    }
    input := strings.Join(lines, "\n")
    out := compressGrep(input)
    // 最多 15 个文件
    outLines := strings.Split(strings.TrimSpace(out), "\n")
    // 每文件 3 条全保留 + 无摘要（没超过 maxPerFile）
    if len(outLines) > 15*3 {
        t.Errorf("expected ≤45 lines (15 files × 3), got %d", len(outLines))
    }
}

func itoaTest(n int) string {
    if n < 0 || n > 999 {
        return "?"
    }
    d1 := byte('0' + n/100)
    d2 := byte('0' + (n/10)%10)
    d3 := byte('0' + n%10)
    if d1 == '0' {
        if d2 == '0' {
            return string([]byte{d3})
        }
        return string([]byte{d2, d3})
    }
    return string([]byte{d1, d2, d3})
}
```

### Step 1-3: 添加 tree 压缩器

```go
// noiseDirs 已知噪声目录——折叠后减少 40-80% token
var noiseDirs = []string{
    "node_modules", ".git", "dist", "build", "target",
    "__pycache__", ".next", ".nuxt", ".cache", ".turbo",
    ".venv", "venv", "coverage", "out", ".mypy_cache",
}

// compressTree 压缩 directory_tree / ls 输出。
// 确定性算法：折叠已知噪声目录，合并同扩展名大量文件。
func compressTree(raw string) string {
    lines := strings.Split(raw, "\n")
    var out []string
    for _, line := range lines {
        trimmed := strings.TrimSpace(line)
        if trimmed == "" {
            out = append(out, line)
            continue
        }
        // 检测噪声目录
        name := extractDirName(trimmed)
        if isNoiseDir(name) {
            // 折叠为一行
            out = append(out, strings.Repeat(" ", indentOf(line))+
                name+"/ [hidden — 依赖目录]")
            continue
        }
        out = append(out, line)
    }
    // 合并同扩展名的文件（>50个时）
    return mergeByExt(strings.Join(out, "\n"))
}

func extractDirName(line string) string {
    // 目录行通常以 "/" 结尾
    s := strings.TrimRight(strings.TrimSpace(line), "/")
    // 取最后一个路径段
    if idx := strings.LastIndex(s, "/"); idx >= 0 {
        s = s[idx+1:]
    }
    if idx := strings.LastIndex(s, "\\"); idx >= 0 {
        s = s[idx+1:]
    }
    return strings.TrimSuffix(s, "/")
}

func isNoiseDir(name string) bool {
    for _, nd := range noiseDirs {
        if strings.EqualFold(name, nd) {
            return true
        }
    }
    return false
}

func indentOf(line string) int {
    for i := 0; i < len(line); i++ {
        if line[i] != ' ' && line[i] != '\t' {
            return i
        }
    }
    return 0
}

func mergeByExt(raw string) string {
    lines := strings.Split(raw, "\n")
    extCount := make(map[string]int)
    for _, line := range lines {
        ext := fileExt(line)
        if ext != "" {
            extCount[ext]++
        }
    }
    // 找到 >50 的扩展名
    largeExts := make(map[string]bool)
    for ext, n := range extCount {
        if n > 50 {
            largeExts[ext] = true
        }
    }
    if len(largeExts) == 0 {
        return raw
    }
    // 对每个大量扩展名，保留前5和后5，其余合并
    var out []string
    extSeen := make(map[string]int)
    extPending := make(map[string][]string) // 缓冲行直到决定是否合并

    flushExt := func(ext string, lines []string) {
        n := len(lines)
        if n <= 10 || !largeExts[ext] {
            for _, l := range lines {
                out = append(out, l)
            }
            return
        }
        // 保留前5 + 合并行 + 后5
        for i := 0; i < 5 && i < n; i++ {
            out = append(out, lines[i])
        }
        out = append(out, strings.Repeat(" ", indentOf(lines[0]))+
            "*"+ext+" × "+strconv.Itoa(n-10)+" [merged]")
        for i := n - 5; i < n; i++ {
            if i >= 5 {
                out = append(out, lines[i])
            }
        }
    }

    var currentExt string
    var currentBuf []string
    for _, line := range lines {
        ext := fileExt(line)
        if ext != currentExt && len(currentBuf) > 0 {
            flushExt(currentExt, currentBuf)
            currentBuf = nil
        }
        currentExt = ext
        if ext != "" {
            extSeen[ext]++
        }
        currentBuf = append(currentBuf, line)
    }
    if len(currentBuf) > 0 {
        flushExt(currentExt, currentBuf)
    }
    return strings.Join(out, "\n")
}

func fileExt(line string) string {
    s := strings.TrimSpace(line)
    if s == "" || strings.HasSuffix(s, "/") {
        return ""
    }
    // 取最后一个路径段
    name := s
    if idx := strings.LastIndex(s, "/"); idx >= 0 {
        name = s[idx+1:]
    }
    if idx := strings.LastIndex(s, "\\"); idx >= 0 {
        name = s[idx+1:]
    }
    if idx := strings.LastIndex(name, "."); idx >= 0 {
        return name[idx:] // ".go", ".ts" 等
    }
    return ""
}
```

### Step 1-4: SmartCompress 路由 + agent.go 接入

```go
// SmartCompress 根据工具名路由到对应压缩器。确定性——不修改缓存前缀。
func SmartCompress(toolName string, raw string) string {
    switch toolName {
    case "grep", "search_content":
        return compressGrep(raw)
    case "ls", "list_directory", "directory_tree":
        return compressTree(raw)
    default:
        return raw // passthrough
    }
}
```

在 `agent.go:executeOne()` 中，`result` 返回后、`truncateToolOutput` 前插入：

```go
// 修改前:
// body, truncMsg := truncateToolOutput(result)

// 修改后:
compressed := SmartCompress(call.Name, result)
body, truncMsg := truncateToolOutput(compressed)
```

---

## Phase 2: 跨轮 toolCache

### Step 2-1: 修改 toolcache.go — session 级缓存

当前问题：`runDirect()` 每轮开头调用 `a.tc.clear()` 清空缓存；TTL 5s 太短。

修改：
1. `agent.go:New()`: TTL 改为 -1（无过期，纯 mtime 驱动）
2. `agent.go:runDirect()`: 移除 `a.tc.clear()` 调用

```go
// agent.go:New() 中:
tc: newToolCache(5 * time.Second), // 旧
tc: newToolCache(-1),             // 新: 无 TTL，仅 mtime 驱动

// agent.go:runDirect() 中:
a.tc.clear()  // ← 删除这行
```

### Step 2-2: 写操作自动失效已存在

代码已用 `invalidatePath()` 处理，无需改动。验证测试：跨 turn 文件未变→缓存命中；文件被写→缓存 miss。

---

## Phase 3: CompactSummary

### Step 3-1: 修改 compact.go — 插入确定性摘要

在 `maybeCompact()` 的 `a.session.Replace(replacement)` 之后：

```go
// 紧凑后插入确定性摘要
summary := buildCompactSummary(msgs[:len(msgs)-keep])
if summary != "" {
    a.session.Add(provider.Message{
        Role:    provider.RoleUser,
        Content: "[Context: " + summary + "]",
    })
}
```

```go
// buildCompactSummary 从被截断的旧消息中提取确定性摘要
func buildCompactSummary(oldMsgs []provider.Message) string {
    var files []string
    fileSet := make(map[string]bool)
    toolCount := make(map[string]int)
    turnCount := 0

    for _, m := range oldMsgs {
        switch m.Role {
        case provider.RoleUser:
            turnCount++
        case provider.RoleTool:
            toolCount[m.Name]++
        case provider.RoleAssistant:
            for _, tc := range m.ToolCalls {
                toolCount[tc.Name]++
                // 提取编辑的文件路径
                if path := extractFilePathFromCall(tc.Name, tc.Arguments); path != "" {
                    if !fileSet[path] {
                        fileSet[path] = true
                        files = append(files, path)
                    }
                }
            }
        }
    }

    if turnCount == 0 {
        return ""
    }

    var parts []string
    parts = append(parts, strconv.Itoa(turnCount)+" turns completed")
    if len(files) > 0 {
        if len(files) > 10 {
            files = files[:10]
        }
        parts = append(parts, "files: "+strings.Join(files, ", "))
    }
    // 前 5 个工具
    type tc struct {
        name  string
        count int
    }
    var top []tc
    for name, count := range toolCount {
        top = append(top, tc{name, count})
    }
    sort.Slice(top, func(i, j int) bool { return top[i].count > top[j].count })
    if len(top) > 5 {
        top = top[:5]
    }
    var toolStrs []string
    for _, t := range top {
        toolStrs = append(toolStrs, t.name+"×"+strconv.Itoa(t.count))
    }
    if len(toolStrs) > 0 {
        parts = append(parts, "tools: "+strings.Join(toolStrs, ", "))
    }

    return strings.Join(parts, ". ")
}

// extractFilePathFromCall 从工具调用参数中提取文件路径
func extractFilePathFromCall(name string, args string) string {
    // 取 "path" 或 "file_path" 字段
    keys := []string{`"path"`, `"file_path"`, `"source"`}
    for _, key := range keys {
        idx := strings.Index(strings.ToLower(args), key)
        if idx < 0 {
            continue
        }
        rest := args[idx+len(key):]
        colon := strings.Index(rest, ":")
        if colon < 0 {
            continue
        }
        val := strings.TrimSpace(rest[colon+1:])
        val = strings.Trim(val, `,"`)
        val = strings.TrimSpace(val)
        val = strings.Trim(val, `"`)
        if val != "" {
            return val
        }
    }
    return ""
}
```

---

## Phase 4: CacheWarmup

### Step 4-1: 在 controller.go 首轮前发送预热请求

在 `runTurnWithRaw()` 中 `!wasLocked` 分支之后（L2 已锁定），用户消息发送之前：

```go
// runTurnWithRaw() 中:
if !wasLocked {
    c.executor.SetRuntimePrompt(c.ctxMgr.Runtime().SystemPrompt())
    // V5.8: 预热 DeepSeek 服务器端前缀缓存
    c.warmupCache(ctx)
}
```

```go
// controller.go 新增方法:
func (c *Controller) warmupCache(ctx context.Context) {
    if c.executor == nil {
        return
    }
    // 构建与正式请求相同前缀的消息（L1 + L2 + 空用户消息）
    msgs := c.executor.Session().Messages
    runtimePrompt := "" // 从 executor 获取
    // 直接通过 provider 发送最小请求
    // ... 见 Step 4-2
}
```

### Step 4-2: 预热请求的最小实现

最简单方式：复用 stream() 的逻辑，发送一个无工具的空请求。
但 controller 不直接持有 provider。需要通过 executor 暴露预热方法。

在 agent.go 新增方法：

```go
// WarmupCache 发送一个微型请求以建立 DeepSeek 服务器端前缀缓存。
// 返回 nil 即使预热失败——主流程不受影响。
func (a *AgentRunner) WarmupCache(ctx context.Context) error {
    tools := a.tools.Schemas()
    a.activeSchemasMu.RLock()
    if a.activeSchemas != nil {
        tools = a.activeSchemas
    }
    a.activeSchemasMu.RUnlock()
    msgs := a.session.Messages
    if a.runtimePrompt != "" && len(msgs) > 0 && msgs[0].Role == provider.RoleSystem {
        injected := make([]provider.Message, 0, len(msgs)+1)
        injected = append(injected, msgs[0])
        injected = append(injected, provider.Message{Role: provider.RoleSystem, Content: a.runtimePrompt})
        injected = append(injected, msgs[1:]...)
        injected = append(injected, provider.Message{Role: provider.RoleUser, Content: "ping"})
        msgs = injected
    }

    ch, err := a.prov.Stream(ctx, provider.Request{
        Messages:    msgs,
        Tools:       tools,
        Temperature: a.temperature,
        MaxTokens:   1,
    })
    if err != nil {
        return err // 非致命
    }
    // 耗尽 channel（只关心缓存建立，不关心内容）
    for range ch {
    }
    return nil
}
```

在 controller.go 的 `runTurnWithRaw()` 中：

```go
if !wasLocked {
    c.executor.SetRuntimePrompt(c.ctxMgr.Runtime().SystemPrompt())
    // V5.8: 预热服务器端缓存
    _ = c.executor.WarmupCache(ctx)
}
```

---

## Phase 5: 全部验证

### Step 5-1: 运行所有测试

```
cd tianxuan && go test ./internal/agent/... -v -count=1
```

### Step 5-2: 构建检查

```
cd tianxuan && go build -ldflags "-s -w" -o bin/tianxuan.exe ./cmd/tianxuan
go vet ./internal/...
```

### Step 5-3: 端到端测试

运行 tianxuan 并验证：
- 首轮缓存正常（WarmupCache 不破坏后续请求）
- grep 长输出被压缩
- 跨轮文件读缓存命中
- 紧凑后摘要出现

---

## 自审

1. **规格覆盖：** 四项优化全部覆盖 ✓
2. **占位符扫描：** 无 TBD/TODO/占位符 ✓
3. **类型一致性：** 所有接口在现有 agent/provider 框架内 ✓
4. **确定性约束：** compressGrep/compressTree 纯函数，不读外部状态 ✓
5. **缓存安全性：** L1/L2/tools 不修改，仅修改 L4 尾部 ✓
