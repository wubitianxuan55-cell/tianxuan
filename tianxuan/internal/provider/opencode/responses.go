package opencode

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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tianxuan/internal/crash"
	"tianxuan/internal/provider"
)

// clientPool shares HTTP connections per base URL, matching the openai
// provider's pool so repeated calls reuse keep-alive sockets.
var (
	clientPool   = make(map[string]*http.Client)
	clientPoolMu sync.Mutex
)

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

// responsesClient speaks the OpenAI Responses protocol (POST /responses, SSE
// streaming). Zen serves GPT/Grok models on this endpoint; the gateway
// synthesises the standard Responses SSE lifecycle from upstream chunks.
type responsesClient struct {
	name    string
	apiKey  string
	keyEnv  string // api_key_env name, surfaced in auth errors
	authed  atomic.Bool
	baseURL string
	model   string
	effort  string // reasoning_effort
	http    *http.Client
}

func newResponsesClient(cfg provider.Config) (*responsesClient, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("opencode: model is required for provider %q", cfg.Name)
	}
	name := cfg.Name
	if name == "" {
		name = "opencode"
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	keyEnv, _ := cfg.Extra["api_key_env"].(string)
	effort, _ := cfg.Extra["effort"].(string)
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "auto" {
		effort = ""
	}
	base := strings.TrimRight(baseURL, "/")
	return &responsesClient{
		name:    name,
		apiKey:  cfg.APIKey,
		keyEnv:  keyEnv,
		baseURL: base,
		model:   cfg.Model,
		effort:  effort,
		http:    getSharedClient(base),
	}, nil
}

func (c *responsesClient) Name() string { return c.name }

func (c *responsesClient) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	body, err := json.Marshal(c.buildRequest(req))
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", c.name, err)
	}
	resp, err := c.sendWithRetry(ctx, body)
	if err != nil {
		return nil, err
	}
	out := make(chan provider.Chunk, 16)
	go c.readStream(ctx, resp, out)
	return out, nil
}

// sendWithRetry POSTs the request and returns the streaming response, retrying
// the connection+header phase on transient errors and retryable statuses with
// exponential backoff (same policy as the openai provider). Once the response
// is streaming, mid-stream failures surface as ChunkError without retry.
func (c *responsesClient) sendWithRetry(ctx context.Context, body []byte) (*http.Response, error) {
	policy := provider.DefaultRetryPolicy()
	rlPolicy := provider.RateLimitRetryPolicy()
	maxAttempts := policy.MaxAttempts
	if rlPolicy.MaxAttempts > maxAttempts {
		maxAttempts = rlPolicy.MaxAttempts
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := policy.Backoff
			if isRateLimit(lastErr) {
				backoff = rlPolicy.Backoff
			}
			if err := backoff.Sleep(ctx, attempt-1); err != nil {
				return nil, err
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
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
			c.authed.Store(true)
			return resp, nil
		}
		msg, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			msg = []byte(fmt.Sprintf("(could not read error body: %v)", readErr))
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			authErr := &provider.AuthError{Provider: c.name, KeyEnv: c.keyEnv, Status: resp.StatusCode}
			if c.authed.Load() && attempt < 2 {
				lastErr = authErr
				continue
			}
			return nil, authErr
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

// httpStatusError carries an HTTP status so shared retry classification works
// without string matching.
type httpStatusError struct {
	name string
	code int
	body string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("%s: status %d: %s", e.name, e.code, e.body)
}

func (e *httpStatusError) HTTPStatus() int { return e.code }

func isRateLimit(err error) bool {
	if err == nil {
		return false
	}
	var se *httpStatusError
	return errors.As(err, &se) && se.code == http.StatusTooManyRequests
}

// --- OpenAI Responses wire protocol ---

type responsesRequest struct {
	Model           string          `json:"model"`
	Input           []inputItem     `json:"input"`
	Tools           []responsesTool `json:"tools,omitempty"`
	Stream          bool            `json:"stream"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	Temperature     float64         `json:"temperature,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
}

// inputItem is one item of the Responses `input` array. Message items carry
// role+content; function_call / function_call_output carry the tool fields.
type inputItem struct {
	Type      string        `json:"type,omitempty"` // message | function_call | function_call_output
	Role      string        `json:"role,omitempty"`
	Content   []contentPart `json:"content,omitempty"`
	CallID    string        `json:"call_id,omitempty"`
	Name      string        `json:"name,omitempty"`
	Arguments string        `json:"arguments,omitempty"`
	Output    string        `json:"output,omitempty"`
}

type contentPart struct {
	Type string `json:"type"` // input_text | output_text
	Text string `json:"text"`
}

type responsesTool struct {
	Type        string          `json:"type"` // function
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// buildRequest translates the chat-style provider.Request into Responses wire
// format. Tool pairing is repaired first (same guard as the openai provider);
// assistant tool_calls expand into function_call items and their results into
// function_call_output items, so the multi-turn history stays valid.
func (c *responsesClient) buildRequest(req provider.Request) responsesRequest {
	out := responsesRequest{
		Model:           c.model,
		Stream:          true,
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		ReasoningEffort: c.effort,
	}
	for _, m := range provider.SanitizeToolPairing(req.Messages) {
		switch m.Role {
		case provider.RoleSystem:
			out.Input = append(out.Input, inputItem{
				Role:    "system",
				Content: []contentPart{{Type: "input_text", Text: m.Content}},
			})
		case provider.RoleUser:
			out.Input = append(out.Input, inputItem{
				Role:    "user",
				Content: []contentPart{{Type: "input_text", Text: m.Content}},
			})
		case provider.RoleAssistant:
			if m.Content != "" {
				out.Input = append(out.Input, inputItem{
					Role:    "assistant",
					Content: []contentPart{{Type: "output_text", Text: m.Content}},
				})
			}
			for _, tc := range m.ToolCalls {
				out.Input = append(out.Input, inputItem{
					Type:      "function_call",
					CallID:    tc.ID,
					Name:      tc.Name,
					Arguments: tc.Arguments,
				})
			}
		case provider.RoleTool:
			out.Input = append(out.Input, inputItem{
				Type:   "function_call_output",
				CallID: m.ToolCallID,
				Output: m.Content,
			})
		}
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, responsesTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	return out
}

// --- SSE parsing ---

type streamEvent struct {
	Type        string         `json:"type"`
	Delta       string         `json:"delta"`
	OutputIndex int            `json:"output_index"`
	Item        *responsesItem `json:"item"`
	Response    *responsesMeta `json:"response"`
	Error       *responsesErr  `json:"error"`
}

type responsesItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type responsesMeta struct {
	Status string        `json:"status"`
	Usage  *wireUsage    `json:"usage"`
	Error  *responsesErr `json:"error"`
}

type responsesErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type wireUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func normaliseUsage(u *wireUsage) *provider.Usage {
	hit, reasoning := 0, 0
	if u.InputTokensDetails != nil {
		hit = u.InputTokensDetails.CachedTokens
	}
	if u.OutputTokensDetails != nil {
		reasoning = u.OutputTokensDetails.ReasoningTokens
	}
	return &provider.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
		CacheHitTokens:   hit,
		ReasoningTokens:  reasoning,
	}
}

func (c *responsesClient) readStream(ctx context.Context, resp *http.Response, out chan<- provider.Chunk) {
	defer resp.Body.Close()
	defer close(out)

	idleDone := make(chan struct{})
	defer close(idleDone)
	go func() {
		defer crash.Recover("opencode-responses-body-close")
		select {
		case <-ctx.Done():
			resp.Body.Close()
		case <-idleDone:
		}
	}()

	acc := map[int]*provider.ToolCall{}
	started := map[int]bool{}
	var order []int

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Reasoning-capable models (GPT/Grok/DeepSeek) may pause the stream for
	// tens of seconds with no frames; mirror the openai provider's idle guard.
	const idleNoticeTimeout = 60 * time.Second
	const idleHardTimeout = 120 * time.Second
	var lastDataNano atomic.Int64
	lastDataNano.Store(time.Now().UnixNano())
	keepaliveSent := false
	go func() {
		defer crash.Recover("opencode-responses-idle-timer")
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-idleDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(time.Unix(0, lastDataNano.Load()))
				if elapsed > idleHardTimeout {
					resp.Body.Close()
					return
				}
				if elapsed > idleNoticeTimeout && !keepaliveSent {
					keepaliveSent = true
					out <- provider.Chunk{Type: provider.ChunkReasoning, Text: "[reasoning in progress...]"}
				}
			}
		}
	}()

	for scanner.Scan() {
		lastDataNano.Store(time.Now().UnixNano())
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var ev streamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			out <- provider.Chunk{Type: provider.ChunkError, Err: fmt.Errorf("%s: decode stream: %w", c.name, err)}
			return
		}
		switch ev.Type {
		case "error":
			msg := "unknown error"
			if ev.Error != nil && ev.Error.Message != "" {
				msg = ev.Error.Message
			}
			out <- provider.Chunk{Type: provider.ChunkError, Err: fmt.Errorf("%s: %s", c.name, msg)}
			return
		case "response.failed":
			if ev.Response != nil && ev.Response.Error != nil {
				out <- provider.Chunk{Type: provider.ChunkError, Err: fmt.Errorf("%s: %s", c.name, ev.Response.Error.Message)}
				return
			}
		case "response.output_text.delta":
			if ev.Delta != "" {
				out <- provider.Chunk{Type: provider.ChunkText, Text: ev.Delta}
			}
		case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
			if ev.Delta != "" {
				out <- provider.Chunk{Type: provider.ChunkReasoning, Text: ev.Delta}
			}
		case "response.output_item.added":
			if ev.Item != nil && ev.Item.Type == "function_call" {
				idx := ev.OutputIndex
				if _, exists := acc[idx]; !exists {
					order = append(order, idx)
				}
				acc[idx] = &provider.ToolCall{ID: ev.Item.CallID, Name: ev.Item.Name}
				if ev.Item.Name != "" && !started[idx] {
					started[idx] = true
					out <- provider.Chunk{Type: provider.ChunkToolCallStart, ToolCall: &provider.ToolCall{ID: ev.Item.CallID, Name: ev.Item.Name}}
				}
			}
		case "response.function_call_arguments.delta":
			idx := ev.OutputIndex
			tc, exists := acc[idx]
			if !exists {
				tc = &provider.ToolCall{}
				acc[idx] = tc
				order = append(order, idx)
			}
			tc.Arguments += ev.Delta
		case "response.output_item.done":
			if ev.Item != nil && ev.Item.Type == "function_call" {
				idx := ev.OutputIndex
				tc, exists := acc[idx]
				if !exists {
					tc = &provider.ToolCall{}
					acc[idx] = tc
					order = append(order, idx)
				}
				if tc.ID == "" {
					tc.ID = ev.Item.CallID
				}
				if tc.Name == "" {
					tc.Name = ev.Item.Name
				}
				if ev.Item.Arguments != "" {
					tc.Arguments = ev.Item.Arguments
				}
				if !started[idx] && tc.Name != "" {
					started[idx] = true
					out <- provider.Chunk{Type: provider.ChunkToolCallStart, ToolCall: &provider.ToolCall{ID: tc.ID, Name: tc.Name}}
				}
			}
		case "response.completed":
			if ev.Response != nil && ev.Response.Usage != nil {
				out <- provider.Chunk{Type: provider.ChunkUsage, Usage: normaliseUsage(ev.Response.Usage)}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		out <- provider.Chunk{Type: provider.ChunkError, Err: &provider.StreamInterruptedError{Err: fmt.Errorf("%s: read stream: %w", c.name, err)}}
		return
	}

	sort.Ints(order)
	for _, idx := range order {
		tc := acc[idx]
		if tc.ID == "" {
			tc.ID = fmt.Sprintf("call_%d", idx)
		}
		out <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: tc}
	}
	out <- provider.Chunk{Type: provider.ChunkDone}
}
