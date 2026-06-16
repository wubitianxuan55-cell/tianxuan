package agent

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ─── V5.13a: ParamStormBreaker (Kun tool-storm-breaker.ts 移植) ──────────

// ParamStormOptions 控制参数级风暴断路器行为。
type ParamStormOptions struct {
	WindowSize  int      // 滑动窗口大小，默认 8
	Threshold   int      // 触发抑制的阈值，默认 3
	ExemptTools []string // 豁免工具名称列表
}

// ParamStormResult 是 Inspect 的返回值。
type ParamStormResult struct {
	Suppress bool   // 是否应抑制此次调用
	Reason   string // 抑制原因（仅 Suppress=true 时有值）
}

type recentParamCall struct {
	name     string
	args     string // canonicalized JSON
	readOnly bool
}

// ParamStormBreaker 检测参数级重复工具调用，防止浪费 API 调用。
//
// 与现有 StormBreaker 不同：
//   - StormBreaker：基于错误签名，检测重复失败（死亡螺旋）
//   - ParamStormBreaker：基于 (toolName, canonicalArgs)，检测重复调用（参数级）
//
// 两者互补——前者事后反应，后者事前预防。
// 线程安全——Inspect/Reset 可并发调用。
type ParamStormBreaker struct {
	mu          sync.Mutex
	windowSize  int
	threshold   int
	exemptTools map[string]bool
	recent      []recentParamCall
}

const defaultParamWindowSize = 8
const defaultParamThreshold = 3

// NewParamStormBreaker 创建参数级风暴断路器。
func NewParamStormBreaker(opts ParamStormOptions) *ParamStormBreaker {
	ws := opts.WindowSize
	if ws <= 0 {
		ws = defaultParamWindowSize
	}
	th := opts.Threshold
	if th <= 0 {
		th = defaultParamThreshold
	}
	exempt := make(map[string]bool, len(opts.ExemptTools))
	for _, name := range opts.ExemptTools {
		exempt[name] = true
	}
	return &ParamStormBreaker{
		windowSize:  ws,
		threshold:   th,
		exemptTools: exempt,
	}
}

// Inspect 检查一次工具调用是否应被抑制。
// readOnly 为 true 表示只读工具。
func (p *ParamStormBreaker) Inspect(toolName string, argsJSON string, readOnly bool) ParamStormResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 豁免工具永远不抑制
	if p.exemptTools[toolName] {
		return ParamStormResult{}
	}

	canonical := canonicalizeArgs(argsJSON)

	// 写入操作清零只读历史（Kun 行为：写入后的只读调用上下文已改变）
	if !readOnly {
		p.clearReadOnlyLocked()
	}

	// 计数相同调用
	count := 0
	for _, entry := range p.recent {
		if entry.name == toolName && entry.args == canonical {
			count++
		}
	}

	if count >= p.threshold-1 {
		return ParamStormResult{
			Suppress: true,
			Reason: fmt.Sprintf(
				"%s was called with identical arguments %d times in this turn; "+
					"repeat-loop guard suppressed the duplicate. "+
					"Choose a narrower query or explain why another identical call is needed.",
				toolName, count+1),
		}
	}

	// 记录此次调用
	p.recent = append(p.recent, recentParamCall{
		name:     toolName,
		args:     canonical,
		readOnly: readOnly,
	})
	// 滑动窗口
	for len(p.recent) > p.windowSize {
		p.recent = p.recent[1:]
	}

	return ParamStormResult{}
}

// Reset 清空历史记录（新 turn 开始时调用）。
func (p *ParamStormBreaker) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recent = p.recent[:0]
}

func (p *ParamStormBreaker) clearReadOnlyLocked() {
	n := 0
	for _, entry := range p.recent {
		if !entry.readOnly {
			p.recent[n] = entry
			n++
		}
	}
	p.recent = p.recent[:n]
}

// canonicalizeArgs 将 JSON 参数字符串规范化为确定的字节流，
// 确保 {"a":1,"b":2} 和 {"b":2,"a":1} 产生相同结果。
func canonicalizeArgs(raw string) string {
	var val any
	if err := json.Unmarshal([]byte(raw), &val); err != nil {
		return raw // 解析失败，保留原样
	}
	canonical := canonicalizeValue(val)
	out, err := json.Marshal(canonical)
	if err != nil {
		return raw
	}
	return string(out)
}


