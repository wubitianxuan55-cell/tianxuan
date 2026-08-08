// Package opencode implements the OpenCode Zen gateway (https://opencode.ai/zen).
// Zen fronts many upstream providers behind one base URL, but serves them over
// different wire protocols: OpenAI-compatible /chat/completions (DeepSeek,
// Kimi, GLM, MiniMax, the free tier), Anthropic /messages (Claude, Qwen) and
// OpenAI Responses /responses (GPT, Grok). A single "opencode" kind therefore
// routes each model to the matching backend instead of forcing users to pick a
// provider per protocol.
//
// The factory delegates to the existing openai / anthropic implementations for
// their protocols and ships a dedicated Responses client (responses.go).
package opencode

import (
	"fmt"
	"strings"

	"tianxuan/internal/provider"
	"tianxuan/internal/provider/anthropic"
	"tianxuan/internal/provider/openai"
)

// defaultBaseURL is the Zen pay-as-you-go endpoint. Users on an OpenCode Go
// subscription can override it with https://opencode.ai/zen/go/v1.
const defaultBaseURL = "https://opencode.ai/zen/v1"

// zenProtocol identifies the wire protocol a Zen model speaks.
type zenProtocol int

const (
	// protocolChat is the OpenAI-compatible chat/completions protocol.
	protocolChat zenProtocol = iota
	// protocolAnthropic is the Anthropic Messages protocol.
	protocolAnthropic
	// protocolResponses is the OpenAI Responses protocol.
	protocolResponses
	// protocolGemini is the Google generateContent protocol.
	protocolGemini
)

// protocolFor maps a Zen model id to its protocol family, mirroring the Zen
// endpoint table (docs.opencode.ai/docs/zen): Claude/Qwen run on /messages,
// GPT/Grok on /responses, Gemini on the Google protocol, everything else on
// /chat/completions.
func protocolFor(model string) zenProtocol {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "claude-"), strings.HasPrefix(m, "qwen"):
		return protocolAnthropic
	case strings.HasPrefix(m, "gpt-"), strings.HasPrefix(m, "grok-"):
		return protocolResponses
	case strings.HasPrefix(m, "gemini-"):
		return protocolGemini
	default:
		return protocolChat
	}
}

func init() {
	provider.Register("opencode", New)
}

// New builds the Zen provider for one model, delegating to the backend that
// speaks that model's wire protocol. base_url is optional and defaults to the
// Zen gateway, so a bare model + key config works without copying endpoints.
func New(cfg provider.Config) (provider.Provider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	switch protocolFor(cfg.Model) {
	case protocolAnthropic:
		// The anthropic provider appends "/v1/messages" itself, but Zen's base
		// URL already ends in /v1 (https://opencode.ai/zen/v1) — strip it so the
		// request lands on /zen/v1/messages instead of /zen/v1/v1/messages.
		cfg.BaseURL = strings.TrimSuffix(strings.TrimRight(cfg.BaseURL, "/"), "/v1")
		return anthropic.New(cfg)
	case protocolResponses:
		return newResponsesClient(cfg)
	case protocolGemini:
		return nil, fmt.Errorf("opencode: model %q uses the Google (Gemini) protocol, which tianxuan does not support yet; pick a chat/completions, Anthropic, or Responses model from https://opencode.ai/zen/v1/models", cfg.Model)
	default:
		return openai.New(cfg)
	}
}
