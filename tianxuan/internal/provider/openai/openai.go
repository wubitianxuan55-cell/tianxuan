package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"strings"
	"time"

	"tianxuan/internal/provider"
)


// clientPool 按 baseURL 共享 HTTP 连接池，减少 TCP 建连开销。
var (
	clientPool   = make(map[string]*http.Client)
	clientPoolMu sync.Mutex
)

// getSharedClient 返回 baseURL 共享的 HTTP client。首次创建，之后复用。
func getSharedClient(baseURL string) *http.Client {
	clientPoolMu.Lock()
	defer clientPoolMu.Unlock()
	if c, ok := clientPool[baseURL]; ok {
		return c
	}
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 120 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
		},
	}
	clientPool[baseURL] = c
	return c
}

func init() {
	provider.Register("openai", New)
}


func New(cfg provider.Config) (provider.Provider, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("openai: base_url is required for provider %q", cfg.Name)
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("openai: model is required for provider %q", cfg.Name)
	}
	name := cfg.Name
	if name == "" {
		name = "openai"
	}
	keyEnv, _ := cfg.Extra["api_key_env"].(string) // for actionable auth errors
	effort, _ := cfg.Extra["effort"].(string)
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "auto" {
		effort = ""
	}
	protocol, _ := cfg.Extra["reasoning_protocol"].(string)
	protocol = normalizeReasoningProtocol(protocol)
	deepseek := protocol == "deepseek" || (protocol == "" && IsDeepSeek(cfg.BaseURL))
	minimax := protocol == "" && IsMiniMax(cfg.BaseURL)
	switch {
	case protocol == "none":
		effort = ""
	case deepseek:
		switch effort {
		case "", "off":
			effort = "high"
		case "high", "max":
		default:
			return nil, fmt.Errorf("openai: provider %q uses DeepSeek thinking; effort must be high or max", name)
		}
	case minimax:
		switch effort {
		case "", "auto":
			effort = "adaptive"
		case "adaptive", "disabled":
		default:
			return nil, fmt.Errorf("openai: provider %q uses MiniMax thinking; effort must be adaptive or disabled", name)
		}
	default:
		if effort != "" {
			switch effort {
			case "low", "medium", "high":
			default:
				return nil, fmt.Errorf("openai: provider %q: effort must be low, medium, or high", name)
			}
		}
	}
	return &client{
		name:     name,
		apiKey:   cfg.APIKey,
		keyEnv:   keyEnv,
		baseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		model:    cfg.Model,
		effort:   effort,
		deepseek: deepseek,
		minimax:  minimax,
		http:     getSharedClient(strings.TrimRight(cfg.BaseURL, "/")),
	}, nil
}

type client struct {
	name     string
	apiKey   string
	keyEnv   string // api_key_env name, surfaced in auth errors
	baseURL  string
	model    string
	http     *http.Client
	effort   string // reasoning_effort for OpenAI; thinking.type for MiniMax; "" = auto/provider default
	deepseek bool   // auto-detected: api.deepseek.com or reasoning_protocol=deepseek
	minimax  bool   // auto-detected: *.minimaxi.com (requires thinking.type, not reasoning_effort)
}

func (c *client) Name() string { return c.name }

func (c *client) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	body, err := json.Marshal(c.buildRequest(req))
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", c.name, err)
	}

	resp, err := c.sendWithRetry(ctx, body)
	if err != nil {
		return nil, err
	}

	out := make(chan provider.Chunk, 16) // buffered so readStream can always push final error/close
	go c.readStream(ctx, resp, out)
	return out, nil
}

// sendWithRetry POSTs the request body and returns the streaming response,
// retrying on transient network errors and retryable HTTP statuses (408, 429,
// 5xx) with exponential backoff + jitter. Retries only cover the connection +
// header phase; once we hand the response to readStream, mid-stream failures
// surface as ChunkError without retry, since the model has already started
// emitting tokens we'd otherwise duplicate.
func (c *client) sendWithRetry(ctx context.Context, body []byte) (*http.Response, error) {
	policy := provider.DefaultRetryPolicy()
	rlPolicy := provider.RateLimitRetryPolicy()
	var lastErr error
	rateLimitCount := 0

	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		if attempt > 0 {
			var delay time.Duration
			if isRateLimit(lastErr) {
				rateLimitCount++
				if rateLimitCount >= rlPolicy.MaxAttempts {
					return nil, fmt.Errorf("%s: rate limited after %d attempts", c.name, rateLimitCount)
				}
				delay = rlPolicy.Backoff.Duration(rateLimitCount - 1)
			} else if isServerError(lastErr) {
				delay = policy.Backoff.Duration(attempt - 1)
			} else {
				delay = policy.Backoff.Duration(attempt - 1)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("%s: build request: %w", c.name, err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Accept", "text/event-stream")

		resp, err := c.http.Do(httpReq)
		if err != nil {
			if !provider.IsTransientNetErr(err) {
				return nil, fmt.Errorf("%s: request failed: %w", c.name, err)
			}
			lastErr = fmt.Errorf("%s: request failed: %w", c.name, err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		msg, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			msg = []byte(fmt.Sprintf("(could not read error body: %v)", readErr))
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, &provider.AuthError{Provider: c.name, KeyEnv: c.keyEnv, Status: resp.StatusCode}
		}
		statusErr := &httpStatusError{name: c.name, code: resp.StatusCode, body: strings.TrimSpace(string(msg))}
		if !provider.IsRetryableStatus(resp.StatusCode) {
			return nil, statusErr
		}
		if d := provider.ParseRetryAfter(resp, 120*time.Second); d > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(d):
			}
		}
		lastErr = statusErr
	}
	return nil, lastErr
}

// httpStatusError carries an HTTP status code so retry-classification helpers
// (isRateLimit, isServerError) can inspect it directly instead of matching error
// strings, which would break if the error format changed.
type httpStatusError struct {
	name string
	code int
	body string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("%s: status %d: %s", e.name, e.code, e.body)
}

func normalizeReasoningProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "deepseek", "openai", "none":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func (c *client) buildRequest(req provider.Request) chatRequest {
	// Repair tool-call pairing before sending: an interrupted/resumed history can
	// carry an assistant tool_calls turn whose results never landed, which DeepSeek
	// rejects with a 400 ("must be followed by tool messages …").
	src := provider.SanitizeToolPairing(req.Messages)
	msgs := make([]chatMessage, len(src))
	for i, m := range src {
		cm := chatMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		// DeepSeek thinking 模式下，带 tool_calls 的 assistant turn 必须回传
		// reasoning_content，否则返回 400（"reasoning_content … must be passed back"）。
		// 仅回传 tool_calls 所在的那一轮，不计费的纯文本回复不传——节约 prompt token。
		if c.deepseek && m.Role == provider.RoleAssistant && len(m.ToolCalls) > 0 {
			cm.ReasoningContent = m.ReasoningContent
		}
		for _, tc := range m.ToolCalls {
			wire := chatToolCall{ID: tc.ID, Type: "function"}
			wire.Function.Name = tc.Name
			wire.Function.Arguments = tc.Arguments
			cm.ToolCalls = append(cm.ToolCalls, wire)
		}
		msgs[i] = cm
	}

	var tools []chatTool
	for _, t := range req.Tools {
		tools = append(tools, chatTool{
			Type:     "function",
			Function: chatFunction{Name: t.Name, Description: t.Description, Parameters: t.Parameters},
		})
	}

	out := chatRequest{
		Model:           c.model,
		Messages:        msgs,
		Tools:           tools,
		Stream:          true,
		StreamOptions:   &streamOptions{IncludeUsage: true},
		Temperature:     req.Temperature,
		MaxTokens:       req.MaxTokens,
		ReasoningEffort: c.effort,
	}
	switch {
	case c.deepseek:
		// DeepSeek 的思维链由 thinking.type=enabled 控制（始终开启），
		// reasoning_effort 调节推理深度。我们绝不对 DeepSeek 关闭 thinking。
		out.Thinking = &thinkingMode{Type: "enabled"}
	case c.minimax:
		// MiniMax M3 使用单一的 thinking.type 字段，合法值为
		// "adaptive"（默认，开启）和 "disabled"（关闭）。
		// M3 没有推理深度调节，所以 reasoning_effort 全部省略。
		t := c.effort
		if t == "" {
			t = "adaptive"
		}
		out.Thinking = &thinkingMode{Type: t}
		out.ReasoningEffort = ""
	}
	return out
}

// readStream parses the SSE stream, emits text deltas live, accumulates tool-call
// fragments internally, and emits complete ToolCalls (by index) when done. Each
// call also gets a ChunkToolCallStart the moment its name is known, so a frontend
// can show the tool card while the arguments are still streaming.
func (c *client) readStream(ctx context.Context, resp *http.Response, out chan<- provider.Chunk) {
	defer resp.Body.Close()
	defer close(out)

	// Close the response body when the context is canceled so scanner.Scan()
	// unblocks instead of hanging indefinitely on a stalled connection.
	go func() {
		<-ctx.Done()
		resp.Body.Close()
	}()

	acc := map[int]*provider.ToolCall{}
	started := map[int]bool{}
	var order []int
	var lastFinishReason string
	var think thinkSplitter

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Idle timeout: DeepSeek's SSE stream may pause for 30-60s during reasoning
	// with no keep-alive frames. We track the last data receipt and:
	//   - at 60s idle: emit a keepalive notice so the user knows reasoning is in progress
	//   - at 120s idle: forcibly close the body (hard timeout for true disconnects)
	const idleNoticeTimeout = 60 * time.Second
	const idleHardTimeout   = 120 * time.Second
	lastData := time.Now()
	keepaliveSent := false
	idleDone := make(chan struct{})
	defer close(idleDone)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-idleDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(lastData)
				if elapsed > idleHardTimeout {
					resp.Body.Close()
					return
				}
				if elapsed > idleNoticeTimeout && !keepaliveSent {
					keepaliveSent = true
					out <- provider.Chunk{Type: provider.ChunkReasoning, Text: "[reasoning in progress …]"}
				}
			}
		}
	}()
	for scanner.Scan() {
		lastData = time.Now()
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var sr streamResponse
		if err := json.Unmarshal([]byte(data), &sr); err != nil {
			out <- provider.Chunk{Type: provider.ChunkError, Err: fmt.Errorf("%s: decode stream: %w", c.name, err)}
			return
		}
		if sr.Error != nil {
			out <- provider.Chunk{Type: provider.ChunkError, Err: fmt.Errorf("%s: %s", c.name, sr.Error.Message)}
			return
		}
		if len(sr.Choices) > 0 && sr.Choices[0].FinishReason != nil && *sr.Choices[0].FinishReason != "" {
			lastFinishReason = *sr.Choices[0].FinishReason
		}
		if sr.Usage != nil {
			u := normaliseUsage(sr.Usage)
			u.FinishReason = lastFinishReason
			out <- provider.Chunk{Type: provider.ChunkUsage, Usage: u}
		}
		if len(sr.Choices) == 0 {
			continue
		}

		delta := sr.Choices[0].Delta
		if delta.ReasoningContent != "" {
			out <- provider.Chunk{Type: provider.ChunkReasoning, Text: delta.ReasoningContent}
		}
		if delta.Content != "" {
			r, txt := think.push(delta.Content)
			if r != "" {
				out <- provider.Chunk{Type: provider.ChunkReasoning, Text: r}
			}
			if txt != "" {
				out <- provider.Chunk{Type: provider.ChunkText, Text: txt}
			}
		}
		for _, tc := range delta.ToolCalls {
			cur, ok := acc[tc.Index]
			if !ok {
				cur = &provider.ToolCall{}
				acc[tc.Index] = cur
				order = append(order, tc.Index)
			}
			if tc.ID != "" {
				cur.ID = tc.ID
			}
			if tc.Function.Name != "" {
				cur.Name = tc.Function.Name
			}
			cur.Arguments += tc.Function.Arguments
			// Signal the call's start the moment its name is known, so a frontend
			// can show the tool card immediately rather than only after its
			// (possibly large) arguments finish streaming.
			if !started[tc.Index] && cur.Name != "" {
				started[tc.Index] = true
				out <- provider.Chunk{Type: provider.ChunkToolCallStart, ToolCall: &provider.ToolCall{ID: cur.ID, Name: cur.Name}}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		out <- provider.Chunk{Type: provider.ChunkError, Err: fmt.Errorf("%s: read stream: %w", c.name, err)}
		return
	}

	if r, txt := think.flush(); r != "" || txt != "" {
		if r != "" {
			out <- provider.Chunk{Type: provider.ChunkReasoning, Text: r}
		}
		if txt != "" {
			out <- provider.Chunk{Type: provider.ChunkText, Text: txt}
		}
	}

	sort.Ints(order)
	for _, idx := range order {
		tc := acc[idx]
		if tc.ID == "" {
			// Some OpenAI-compatible gateways stream tool calls by index with no id.
			// Synthesize a stable one so the result can be paired back to its call —
			// an empty tool_call_id collapses multi-tool turns downstream.
			tc.ID = fmt.Sprintf("call_%d", idx)
		}
		out <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: tc}
	}
	out <- provider.Chunk{Type: provider.ChunkDone}
}

// normaliseUsage folds the two cache-hit shapes the OpenAI-compatible ecosystem
// uses into a single Usage: DeepSeek puts prompt_cache_{hit,miss}_tokens at the
// top of usage; OpenAI and MiMo put it nested under prompt_tokens_details.
// Whichever side reports non-zero wins; miss is derived when only hit is given.
// Reasoning tokens land in completion_tokens_details on thinking-mode models.
func normaliseUsage(u *wireUsage) *provider.Usage {
	hit := u.PromptCacheHitTokens
	miss := u.PromptCacheMissTokens
	if hit == 0 && u.PromptTokensDetails != nil {
		hit = u.PromptTokensDetails.CachedTokens
	}
	if miss == 0 && hit > 0 && u.PromptTokens > hit {
		miss = u.PromptTokens - hit
	}
	reasoning := 0
	if u.CompletionTokensDetails != nil {
		reasoning = u.CompletionTokensDetails.ReasoningTokens
	}
	return &provider.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
		CacheHitTokens:   hit,
		CacheMissTokens:  miss,
		ReasoningTokens:  reasoning,
	}
}

// --- OpenAI-compatible wire protocol ---

type thinkingMode struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model           string         `json:"model"`
	Messages        []chatMessage  `json:"messages"`
	Tools           []chatTool     `json:"tools,omitempty"`
	Stream          bool           `json:"stream"`
	StreamOptions   *streamOptions `json:"stream_options,omitempty"`
	Temperature     float64        `json:"temperature,omitempty"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	Thinking        *thinkingMode  `json:"thinking,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role string `json:"role"`
	// content is always serialized, even when empty: an assistant turn that is
	// pure tool_calls (no preamble text) has empty content, and DeepSeek's
	// strict deserializer rejects a message missing the field ("missing field
	// `content`"). An empty string satisfies presence and is accepted by every
	// OpenAI-compatible backend for all roles (unlike null, which some reject
	// for a tool message).
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	Name             string         `json:"name,omitempty"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type chatToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type streamResponse struct {
	Choices []struct {
		Delta struct {
			Content          string         `json:"content"`
			ReasoningContent string         `json:"reasoning_content"`
			ToolCalls        []chatToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// wireUsage covers both DeepSeek's top-level cache fields and the
// OpenAI/MiMo nested details — normaliseUsage chooses whichever side
// reports values.
type wireUsage struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
	PromptTokensDetails   *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

// isRateLimit 判断错误是否来自 429（Rate Limit）。
// 优先使用结构化错误（httpStatusError），降级为字符串匹配以兼容非 HTTP 错误。
func isRateLimit(err error) bool {
	if err == nil {
		return false
	}
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.code == http.StatusTooManyRequests
	}
	s := err.Error()
	return strings.Contains(s, "429") || strings.Contains(s, "rate limit") || strings.Contains(s, "too many")
}

// isServerError 判断错误是否来自 5xx（服务端错误）。
// 优先使用结构化错误（httpStatusError），降级为字符串匹配以兼容非 HTTP 错误。
func isServerError(err error) bool {
	if err == nil {
		return false
	}
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.code >= 500 && se.code <= 599
	}
	s := err.Error()
	return strings.Contains(s, "500") || strings.Contains(s, "502") ||
		strings.Contains(s, "503") || strings.Contains(s, "504")
}
