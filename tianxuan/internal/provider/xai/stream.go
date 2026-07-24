package xai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"tianxuan/internal/provider"
)

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
			DialContext: (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
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

// ── Provider ───────────────────────────────────────────────────────

type xaiStreamer struct {
	name    string
	baseURL string
	model   string
	tm      *tokenManager
	hc      *http.Client
}

// New creates an XAI streaming provider. cfg.APIKey is optional (falls back to OAuth).
func New(cfg provider.Config) (provider.Provider, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("xai: base_url is required for %q", cfg.Name)
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("xai: model is required for %q", cfg.Name)
	}
	name := cfg.Name
	if name == "" {
		name = "xai"
	}
	tm := newTokenManager(cfg.APIKey)
	slog.Debug("xai: provider created", "name", name, "model", cfg.Model, "hasApiKey", cfg.APIKey != "", "isLoggedIn", tm.IsLoggedIn())
	return &xaiStreamer{
		name:    name,
		baseURL: normalizeBaseURL(cfg.BaseURL),
		model:   cfg.Model,
		tm:      tm,
		hc:      getSharedClient(normalizeBaseURL(cfg.BaseURL)),
	}, nil
}

func (x *xaiStreamer) Name() string { return x.name }

func (x *xaiStreamer) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	body, err := x.buildBody(req)
	if err != nil {
		return nil, err
	}
	token, err := x.tm.getAccessToken()
	if err != nil {
		return nil, &provider.AuthError{Provider: x.name, KeyEnv: "XAI_API_KEY (or OAuth login)", Status: 401}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(x.baseURL, "/")+"/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	resp, err := x.hc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		resp.Body.Close()
		return nil, &provider.AuthError{Provider: x.name, KeyEnv: "XAI_API_KEY (or OAuth login)", Status: resp.StatusCode}
	}
	out := make(chan provider.Chunk, 16)
	go x.readStream(ctx, resp, out)
	return out, nil
}

// ── SSE stream reading ────────────────────────────────────────────

func (x *xaiStreamer) readStream(ctx context.Context, resp *http.Response, out chan<- provider.Chunk) {
	defer close(out)
	defer resp.Body.Close()

	const idleHardTimeout = 120 * time.Second
	reader := bufio.NewReader(resp.Body)
	lastRead := time.Now()

	for {
		if time.Since(lastRead) > idleHardTimeout {
			out <- provider.Chunk{Type: provider.ChunkError, Err: fmt.Errorf("%s: stream idle timeout", x.name)}
			return
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			out <- provider.Chunk{Type: provider.ChunkError, Err: fmt.Errorf("%s: read stream: %w", x.name, err)}
			return
		}
		lastRead = time.Now()
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return
		}
		var sr struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
				// DeepSeek-style cache fields
				PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
				PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
				// OpenAI-style cache field (XAI may use this format)
				PromptTokensDetails *struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
				CompletionTokensDetails *struct {
					ReasoningTokens int `json:"reasoning_tokens"`
				} `json:"completion_tokens_details"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &sr); err != nil {
			continue
		}
		if sr.Usage != nil {
			u := sr.Usage
			cacheHit := u.PromptCacheHitTokens
			// Fallback: OpenAI-style prompt_tokens_details.cached_tokens
			if cacheHit == 0 && u.PromptTokensDetails != nil {
				cacheHit = u.PromptTokensDetails.CachedTokens
			}
			out <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{
				PromptTokens:     u.PromptTokens,
				CompletionTokens: u.CompletionTokens,
				TotalTokens:      u.TotalTokens,
				CacheHitTokens:   cacheHit,
				CacheMissTokens:  max(0, u.PromptTokens-cacheHit),
				ReasoningTokens:  func() int {
					if u.CompletionTokensDetails != nil {
						return u.CompletionTokensDetails.ReasoningTokens
					}
					return 0
				}(),
			}}
		}
		for _, c := range sr.Choices {
			if c.Delta.ReasoningContent != "" {
				out <- provider.Chunk{Type: provider.ChunkReasoning, Text: c.Delta.ReasoningContent}
			}
			if c.Delta.Content != "" {
				out <- provider.Chunk{Type: provider.ChunkText, Text: c.Delta.Content}
			}
			for _, tc := range c.Delta.ToolCalls {
				if tc.ID != "" {
					out <- provider.Chunk{Type: provider.ChunkToolCallStart,
						ToolCall: &provider.ToolCall{ID: tc.ID, Name: tc.Function.Name}}
				}
				if tc.Function.Arguments != "" {
					out <- provider.Chunk{Type: provider.ChunkToolCall,
						ToolCall: &provider.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments}}
				}
			}
			if c.FinishReason != nil && *c.FinishReason != "" {
				out <- provider.Chunk{Type: provider.ChunkDone}
			}
		}
	}
}

// ── Request body ──────────────────────────────────────────────────

func (x *xaiStreamer) buildBody(req provider.Request) ([]byte, error) {
	type msg struct {
		Role             string `json:"role"`
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content,omitempty"`
		ToolCalls        []struct {
			Index    int    `json:"index"`
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls,omitempty"`
		ToolCallID string `json:"tool_call_id,omitempty"`
		Name       string `json:"name,omitempty"`
	}
	msgs := make([]msg, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = msg{Role: string(m.Role), Content: m.Content, ReasoningContent: m.ReasoningContent, ToolCallID: m.ToolCallID, Name: m.Name}
		tcs := make([]struct {
			Index    int    `json:"index"`
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}, len(m.ToolCalls))
		for j, tc := range m.ToolCalls {
			tcs[j].Index = j
			tcs[j].ID = tc.ID
			tcs[j].Type = "function"
			tcs[j].Function.Name = tc.Name
			tcs[j].Function.Arguments = tc.Arguments
		}
		msgs[i].ToolCalls = tcs
	}
	type tool struct {
		Type     string `json:"type"`
		Function struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		} `json:"function"`
	}
	tools := make([]tool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i].Type = "function"
		tools[i].Function.Name = t.Name
		tools[i].Function.Description = t.Description
		tools[i].Function.Parameters = t.Parameters
	}
	cr := map[string]any{
		"model":          x.model,
		"messages":       msgs,
		"stream":         true,
		"stream_options": map[string]bool{"include_usage": true},
		"tools":          tools,
	}
	if req.Temperature > 0 {
		cr["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		cr["max_tokens"] = req.MaxTokens
	}
	return json.Marshal(cr)
}
