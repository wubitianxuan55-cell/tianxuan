// Package provider defines the model-backend abstraction and a registry mapping
// a provider "kind" to a factory. Concrete implementations live in subpackages
// (e.g. provider/openai) and self-register via init(). The core resolves
// providers by kind from config and never hardcodes a specific model.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"tianxuan/internal/nilutil"
)

// Role is the role of a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single conversation message.
type Message struct {
	Role             Role   `json:"role"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"` // assistant: thinking-mode chain-of-thought, round-tripped on multi-turn
	// ReasoningSignature is an opaque, provider-issued proof that ReasoningContent
	// is genuine model output. Anthropic requires the signed thinking block be
	// replayed on the next turn when a tool call followed thinking; providers
	// without signed reasoning (e.g. the openai-compatible ones) leave it empty.
	// Round-tripped alongside ReasoningContent.
	ReasoningSignature string     `json:"reasoning_signature,omitempty"`
	ToolCalls          []ToolCall `json:"tool_calls,omitempty"`   // set by assistant
	ToolCallID         string     `json:"tool_call_id,omitempty"` // links a tool result to its call
	Name               string     `json:"name,omitempty"`         // tool message: tool name
}

// ToolCall is a tool invocation requested by the model. Arguments is raw JSON.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolSchema is a tool definition exposed to the model. Parameters is JSON Schema.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Request is a single completion request.
type Request struct {
	Messages    []Message
	Tools       []ToolSchema
	Temperature float64
	MaxTokens   int
}

// interruptedToolResult stands in for a tool result that never landed 鈥?an
// assistant tool_calls turn whose execution was cut short (interrupt, crash) and
// later resumed. Sending such a turn unanswered trips the OpenAI/DeepSeek 400
// "An assistant message with 'tool_calls' must be followed by tool messages
// responding to each 'tool_call_id'".
const interruptedToolResult = "[no result: the previous turn was interrupted before this tool call completed]"

// SanitizeToolPairing repairs a history so it satisfies the tool-call contract the
// OpenAI-compatible and Anthropic APIs enforce: every assistant tool_calls entry
// must be answered by a following tool message for its id, and a tool message must
// follow such a call. It backfills a placeholder result for any unanswered call
// (so the turn stays intact) and drops orphan tool messages. Well-formed histories
// pass through unchanged (results stay in call order). Callers send the result;
// the stored session keeps the original.
//
// V5.11: 鍗囩骇鑷?Kun model-history-repair.ts锛屾柊澧炴ˉ鎺ユ秷鎭鐞嗐€?
// tool_call 鍜?tool_result 涔嬮棿鐨?assistant 鏂囨湰/reasoning 绛?
// 妗ユ帴娑堟伅涓嶅啀闃绘柇閰嶅鎵弿銆?
func SanitizeToolPairing(msgs []Message) []Message {
	// V10.0 Fast Path: skip repair when no assistant tool_calls or orphan tools.
	needsRepair := false
	for i, m := range msgs {
		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			needsRepair = true
			break
		}
		if m.Role == RoleTool {
			prev := i > 0 && msgs[i-1].Role == RoleAssistant && len(msgs[i-1].ToolCalls) > 0
			if !prev {
				needsRepair = true
				break
			}
		}
	}
	if !needsRepair {
		return msgs
	}

	out := make([]Message, 0, len(msgs))
	for i := 0; i < len(msgs); {
		m := msgs[i]
		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			// 鎵弿 tool results锛岃烦杩囨ˉ鎺ユ秷鎭紙assistant 鏂囨湰绛夛級
			j := i + 1
			var bridge []Message
			for j < len(msgs) && isToolResultOrBridge(msgs[j]) {
				if msgs[j].Role == RoleTool {
					// 鏀堕泦 tool result
				} else {
					// 妗ユ帴娑堟伅鈥斺€斾繚鐣欎絾缁х画鎵弿
					bridge = append(bridge, msgs[j])
				}
				j++
			}
			// 浠庢壂鎻忚寖鍥翠腑鎻愬彇 tool results
			toolResults := extractToolResults(msgs[i+1 : j])
			out = append(out, m)
			out = append(out, pairToolResults(m.ToolCalls, toolResults)...)
			out = append(out, bridge...) // 妗ユ帴娑堟伅鏀惧湪 tool results 涔嬪悗
			i = j
			continue
		}
		if m.Role == RoleTool {
			i++ // orphan tool message (no preceding assistant tool_calls) 鈥?drop
			continue
		}
		out = append(out, m)
		i++
	}
	return out
}

// isToolResultOrBridge reports whether a message is either a tool result
// (should be paired) or a bridge item (assistant text between tool_call
// and tool_result that should not break scanning).
func isToolResultOrBridge(m Message) bool {
	if m.Role == RoleTool {
		return true
	}
	// 妗ユ帴娑堟伅锛歛ssistant 鏂囨湰锛堟棤 tool calls锛夈€乺easoning 绛?
	if m.Role == RoleAssistant && len(m.ToolCalls) == 0 && m.Content != "" {
		return true
	}
	return false
}

// extractToolResults filters only RoleTool messages from a slice.
func extractToolResults(msgs []Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == RoleTool {
			out = append(out, m)
		}
	}
	return out
}

// pairToolResults answers each tool_call with its result, backfilling a
// placeholder for any unanswered one. Distinct non-empty ids pair by id (so
// reordered results re-sort to call order); empty or duplicate ids pair by
// position instead 鈥?some gateways stream tool calls by index with no id, and a
// map keyed on id would collapse those results into one (call order is preserved
// because the loop appends results in call order).
func pairToolResults(calls []ToolCall, avail []Message) []Message {
	out := make([]Message, 0, len(calls))
	if idDistinct(calls) {
		byID := make(map[string]Message, len(avail))
		for _, r := range avail {
			byID[r.ToolCallID] = r
		}
		for _, tc := range calls {
			if r, ok := byID[tc.ID]; ok {
				out = append(out, r)
			} else {
				out = append(out, Message{Role: RoleTool, ToolCallID: tc.ID, Name: tc.Name, Content: interruptedToolResult})
			}
		}
		return out
	}
	for k, tc := range calls {
		if k < len(avail) {
			r := avail[k]
			r.ToolCallID = tc.ID
			out = append(out, r)
		} else {
			out = append(out, Message{Role: RoleTool, ToolCallID: tc.ID, Name: tc.Name, Content: interruptedToolResult})
		}
	}
	return out
}

// idDistinct reports whether every call carries a non-empty id unique within the
// batch 鈥?the condition under which id-keyed pairing is safe.
func idDistinct(calls []ToolCall) bool {
	seen := make(map[string]struct{}, len(calls))
	for _, tc := range calls {
		if tc.ID == "" {
			return false
		}
		if _, dup := seen[tc.ID]; dup {
			return false
		}
		seen[tc.ID] = struct{}{}
	}
	return true
}

// ChunkType identifies the kind of a streamed increment.
type ChunkType int

const (
	ChunkText          ChunkType = iota // text delta
	ChunkReasoning                      // thinking-mode reasoning delta (before the visible answer)
	ChunkToolCallStart                  // a tool call has begun (ToolCall: ID+Name; args still streaming)
	ChunkToolCall                       // one complete tool call
	ChunkUsage                          // token usage for the completion
	ChunkDone                           // completion finished normally
	ChunkError                          // an error occurred
)

// Usage reports token accounting for a completion. Cache hit/miss come from
// either DeepSeek's top-level prompt_cache_{hit,miss}_tokens or the OpenAI/MiMo
// standard prompt_tokens_details.cached_tokens 鈥?the openai provider normalises
// both shapes into these fields. ReasoningTokens is the thinking-mode subset of
// CompletionTokens reported by thinking-capable models. FinishReason carries
// the model's last reported choices[0].finish_reason so the agent can surface
// abnormal terminations ("length", "content_filter", "repetition_truncation").
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CacheHitTokens   int    // prompt tokens served from cache
	CacheMissTokens  int    // prompt tokens not cached
	ReasoningTokens        int    // subset of CompletionTokens spent on chain-of-thought
	SessionCacheHitTokens  int    // cumulative cache hit tokens across the session
	SessionCacheMissTokens int    // cumulative cache miss tokens across the session
	FinishReason     string // "stop", "tool_calls", "length", "content_filter", "repetition_truncation", 鈥?
}

// Pricing is a provider's per-1M-token rates, used to estimate spend. Currency
// is just a display symbol (default "楼"). toml tags let config decode it.
type Pricing struct {
	CacheHit float64 `toml:"cache_hit"` // per 1M cached prompt tokens
	Input    float64 `toml:"input"`     // per 1M uncached prompt tokens
	Output   float64 `toml:"output"`    // per 1M completion tokens
	Currency string  `toml:"currency"`
}

// Cost estimates the spend for a usage record.
func (p *Pricing) Cost(u *Usage) float64 {
	if p == nil || u == nil {
		return 0
	}
	return (float64(u.CacheHitTokens)*p.CacheHit +
		float64(u.CacheMissTokens)*p.Input +
		float64(u.CompletionTokens)*p.Output) / 1e6
}

// Symbol returns the currency display symbol, defaulting to "楼".
func (p *Pricing) Symbol() string {
	if p == nil || p.Currency == "" {
		return "楼"
	}
	return p.Currency
}

// Chunk is a single streamed event. Read the field matching Type.
type Chunk struct {
	Type      ChunkType
	Text      string    // ChunkText, ChunkReasoning
	Signature string    // ChunkReasoning: opaque proof for the reasoning (Anthropic thinking signature), when issued
	ToolCall  *ToolCall // ChunkToolCallStart (ID+Name only), ChunkToolCall (complete)
	Usage     *Usage    // ChunkUsage
	Err       error     // ChunkError
}

// Provider is a chat-capable model backend.
type Provider interface {
	// Name returns the provider instance name, e.g. "deepseek" / "mimo".
	Name() string
	// Stream starts a streaming completion, pushing increments on the channel.
	// Cancelling ctx must abort the underlying request; a closed channel marks
	// the end of the completion.
	Stream(ctx context.Context, req Request) (<-chan Chunk, error)
}

// Config is a resolved provider instance configuration.
type Config struct {
	Name    string         // instance name, e.g. "deepseek"
	BaseURL string         // OpenAI-compatible endpoint
	Model   string         // model id
	APIKey  string         // resolved from api_key_env
	Extra   map[string]any // kind-specific options
}

// AuthError reports that a provider rejected the API key (HTTP 401/403). Its
// message is already user-facing and actionable 鈥?it names the provider and,
// when known, the environment variable the key comes from 鈥?so the CLI can
// surface it verbatim instead of dumping a raw status body. Providers should
// return this (rather than a generic status error) for auth failures.
type AuthError struct {
	Provider  string // the provider instance name, e.g. "deepseek"
	KeyEnv    string // the api_key_env the key is read from, when known
	KeySource string // human-readable description of where the key was configured
	HasKey    bool   // whether a key value was actually present
	Status    int    // the HTTP status (401 or 403)
}

func (e *AuthError) Error() string {
	key := "the API key"
	if e.KeyEnv != "" {
		key = e.KeyEnv
	}
	return fmt.Sprintf("authentication failed for provider %q (HTTP %d): %s is invalid or expired 鈥?update it (in .env or your environment) and retry, or run `tianxuan setup`",
		e.Provider, e.Status, key)
}

// Factory builds a Provider from a resolved Config.
type Factory func(cfg Config) (Provider, error)

var registry = map[string]Factory{}

// Register adds a factory under a kind (e.g. "openai"). Intended for init().
// It panics on a duplicate kind, since that is a compile-time wiring mistake.
func Register(kind string, f Factory) {
	if _, dup := registry[kind]; dup {
		panic("provider: duplicate kind " + kind)
	}
	registry[kind] = f
}

// New instantiates the provider of the given kind.
func New(kind string, cfg Config) (Provider, error) {
	f, ok := registry[kind]
	if !ok {
		return nil, fmt.Errorf("provider: unknown kind %q (registered: %v)", kind, Kinds())
	}
	p, err := f(cfg)
	if err != nil {
		return nil, err
	}
	if nilutil.IsNil(p) {
		return nil, fmt.Errorf("provider: factory %q returned nil provider", kind)
	}
	return p, nil
}

// Kinds returns the registered kinds, sorted.
func Kinds() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// StreamInterruptedError 标记一个可恢复的流传输中断——在调用者已经收到模型输出
// 之后发生。Provider 不能自行重放这些请求，因为那样可能重复可见文本或工具调用；
// Agent 可以注入尾部恢复提示语来替代。
// (Design adopted from DeepSeek-Reasonix-V1.12)
type StreamInterruptedError struct {
	Err error
}

func (e *StreamInterruptedError) Error() string {
	if e == nil || e.Err == nil {
		return "stream interrupted"
	}
	return e.Err.Error()
}

func (e *StreamInterruptedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsStreamInterrupted 检查 error 是否为可恢复的流中断。
func IsStreamInterrupted(err error) bool {
	var interrupted *StreamInterruptedError
	return errors.As(err, &interrupted)
}
