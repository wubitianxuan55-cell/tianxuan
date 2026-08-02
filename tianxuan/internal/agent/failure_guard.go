package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
)

// ─── V10.148: Auto Failure Guard（蒸馏自 Reasonix internal/recovery）───
//
// 宿主侧失败升级决策，与现有 nudge 注入机制（repeat_detect / tool_feedback /
// todo_step_nudge）互补：那些机制提示模型自行调整，本 Guard 在宿主侧硬决策。
//
// 语义（对齐 Reasonix recovery.Decide + rules.QualifyingFailure）：
//   - 只读工具失败是正常诊断信息，不累计；
//   - 同一精确操作（工具名 + 规范化参数指纹）失败 MaxOperationFailures 次后，
//     宿主停止该操作（模型换参数/换方案即视为新操作）；
//   - 回合内累计 MaxEpisodeFailures 次变更/验证失败且无真实进展时，宿主停止
//     回合的变更与验证，只读诊断保持可用；
//   - 成功的变更/验证 = 真实进展，清零回合预算；已停止的精确操作不解除。
//
// 缓存安全：本 Guard 不改变系统提示词与工具 schema（成功路径 L1-L4 前缀
// 完全不变）；失败路径由接线层注入动态 user-tail 消息，与现有 nudge 同模式。

const (
	// MaxOperationFailures 是同一精确操作在一次 Guard 生命周期内允许失败的
	// 次数，达到后宿主停止该操作（Reasonix MaxOperationFailures=3）。
	MaxOperationFailures = 3
	// MaxEpisodeFailures 是回合内无真实进展的累计失败上限，达到后宿主停止
	// 回合的变更/验证（Reasonix MaxEpisodeFailures=6）。
	MaxEpisodeFailures = 6
)

// autoGuardOperationStoppedMsg 在宿主停止一个精确操作后注入（编译期常量，
// 失败路径动态 user-tail，不破坏成功路径的 L1-L4 前缀）。
const autoGuardOperationStoppedMsg = "[system] 宿主已停止该操作：同一操作已连续失败 " +
	"多次且无进展。请更换参数、改用其他工具或调整方案，不要重试同一操作。"

// autoGuardEpisodeStoppedMsg 在宿主暂停回合的变更/验证后注入。
const autoGuardEpisodeStoppedMsg = "[system] 宿主已暂停本回合的变更与验证：连续多次失败" +
	"且无真实进展。请停止写操作，先做只读诊断（read_file/ls/glob/grep），或总结现状结束回合。"

// autoGuardBlockMessage 生成被宿主拒绝的工具结果文本（直接反馈给模型）。
func autoGuardBlockMessage(d GuardDecision) string {
	switch d {
	case GuardDenyOperation:
		return "blocked: [auto guard] this exact operation has failed repeatedly; the host stopped it. Change the arguments or use a different approach."
	case GuardDenyTurn:
		return "blocked: [auto guard] this turn's mutation budget is exhausted after repeated failures without progress; the host paused mutations. Use read-only diagnosis or summarize and finish."
	default:
		return ""
	}
}

// GuardDecision 是 Check 的宿主侧路由结果。
type GuardDecision int

const (
	// GuardAllow 放行执行。
	GuardAllow GuardDecision = iota
	// GuardDenyOperation 拒绝该精确操作（已达操作级失败上限）。
	GuardDenyOperation
	// GuardDenyTurn 拒绝回合内一切变更/验证（回合级失败上限，仅只读可用）。
	GuardDenyTurn
)

// GuardOutcome 是 Observe 的升级信号，接线层据此注入引导消息。
type GuardOutcome int

const (
	// GuardNone 无升级。
	GuardNone GuardOutcome = iota
	// GuardOperationStopped 刚停止一个精确操作。
	GuardOperationStopped
	// GuardEpisodeStopped 刚停止回合的变更/验证。
	GuardEpisodeStopped
)

// FailureGuard 是线程安全的失败升级状态机。一次生命周期对应一个回合：
// AgentRunner.Run 开始时调用 Reset；跨回合不共享升级状态。
type FailureGuard struct {
	mu              sync.Mutex
	opFailures      map[string]int
	stoppedOps      map[string]bool
	episodeFailures int
	episodeStopped  bool
}

// NewFailureGuard 创建空状态机。
func NewFailureGuard() *FailureGuard {
	return &FailureGuard{
		opFailures: make(map[string]int),
		stoppedOps: make(map[string]bool),
	}
}

// Reset 清空全部升级状态（回合开始/新任务时调用）。
func (g *FailureGuard) Reset() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.opFailures = make(map[string]int)
	g.stoppedOps = make(map[string]bool)
	g.episodeFailures = 0
	g.episodeStopped = false
}

// OperationFingerprint 为一次精确操作生成稳定指纹：工具名 + 规范化参数
// （JSON key 顺序无关，避免模型以不同顺序重发同参被误判为新操作）。
func OperationFingerprint(name string, args json.RawMessage) string {
	h := sha256.New()
	h.Write([]byte("tool="))
	h.Write([]byte(strings.TrimSpace(name)))
	h.Write([]byte("\nargs="))
	h.Write([]byte(normalizeJSON(args)))
	return hex.EncodeToString(h.Sum(nil))
}

// Check 在执行前做宿主侧路由。mutates 为 true 表示变更/验证型调用。
func (g *FailureGuard) Check(name string, args json.RawMessage, mutates bool) GuardDecision {
	if g == nil {
		return GuardAllow
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.episodeStopped && mutates {
		return GuardDenyTurn
	}
	if g.stoppedOps[OperationFingerprint(name, args)] {
		return GuardDenyOperation
	}
	return GuardAllow
}

// Observe 在工具实际执行后记录结果。只读调用（mutates=false）一律不累计；
// 变更/验证失败累计升级，成功则清零回合预算（真实进展），但不清除已停止的
// 精确操作。返回升级信号供接线层注入引导消息。
func (g *FailureGuard) Observe(name string, args json.RawMessage, mutates, failed bool) GuardOutcome {
	if g == nil {
		return GuardNone
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !mutates {
		return GuardNone
	}
	if !failed {
		g.episodeFailures = 0
		g.episodeStopped = false
		return GuardNone
	}
	fp := OperationFingerprint(name, args)
	g.opFailures[fp]++
	g.episodeFailures++
	if !g.stoppedOps[fp] && g.opFailures[fp] >= MaxOperationFailures {
		g.stoppedOps[fp] = true
		return GuardOperationStopped
	}
	if !g.episodeStopped && g.episodeFailures >= MaxEpisodeFailures {
		g.episodeStopped = true
		return GuardEpisodeStopped
	}
	return GuardNone
}

// EpisodeStopped 报告回合是否已被宿主停止（供主循环收尾判断）。
func (g *FailureGuard) EpisodeStopped() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.episodeStopped
}

// normalizeJSON 递归规范化 JSON：map key 排序、数组保持顺序、标量原样。
// 非法 JSON 原样返回（指纹退化为原始字节，仍稳定）。
func normalizeJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(normalizeValue(v))
	if err != nil {
		return string(raw)
	}
	return string(b)
}

func normalizeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(t))
		for _, k := range keys {
			out[k] = normalizeValue(t[k])
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalizeValue(e)
		}
		return out
	default:
		return v
	}
}
