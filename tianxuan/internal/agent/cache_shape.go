// Package agent — V8.9: CacheShape provides per-request observability
// for diagnosing DeepSeek prefix-cache behaviour. After every LLM completion,
// a lightweight fingerprint of the message array shape is computed and emitted
// as a notice so the user can correlate shape changes to cache-break events.
//
// Cache safety: this is pure side-channel telemetry. It reads session messages
// and tool schemas but never modifies them. The notice is informational only.
package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"tianxuan/internal/provider"
)

// CacheShape carries the structural fingerprint of one LLM request so the
// user can correlate shape mutations (new tools, system-prompt edits) to
// cache-break events. It is emitted as a Notice after Usage.
type CacheShape struct {
	Kind       string `json:"kind"`        // "agent" | "compact"
	MsgCount   int    `json:"msg_count"`   // total messages in the request
	Roles      string `json:"roles"`       // compact role initials, e.g. "SUATAT..."
	SysHash    string `json:"sys_hash"`    // SHA256 of system message content (first 3 bytes)
	ToolsHash  string `json:"tools_hash"`  // SHA256 of tool-name-list concat (first 3 bytes)
	PrefixHash string `json:"prefix_hash"` // SHA256 of first N msgs concat (first 3 bytes)
	TailHash   string `json:"tail_hash"`   // SHA256 of last 4 msgs concat (first 3 bytes)
}

// computeCacheShape builds a structural fingerprint from the session messages
// and active tool names. It is deterministic: same messages + same tools →
// same shape, so re-requests during fault recovery will produce identical shapes.
func (a *AgentRunner) computeCacheShape(kind string) *CacheShape {
	cs := &CacheShape{Kind: kind}
	msgs := a.session.Messages
	cs.MsgCount = len(msgs)
	if cs.MsgCount == 0 {
		return cs
	}

	// Role pattern: S=system, U=user, A=assistant, T=tool
	var roles strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case provider.RoleSystem:
			roles.WriteByte('S')
		case provider.RoleUser:
			roles.WriteByte('U')
		case provider.RoleAssistant:
			roles.WriteByte('A')
		case provider.RoleTool:
			roles.WriteByte('T')
		default:
			roles.WriteByte('?')
		}
	}
	cs.Roles = roles.String()

	// System hash
	if msgs[0].Role == provider.RoleSystem {
		cs.SysHash = shortHash(msgs[0].Content)
	}

	// Tool-name-list hash
	toolNames := a.tools.Names()
	if len(toolNames) > 0 {
		cs.ToolsHash = shortHash(strings.Join(toolNames, ","))
	}

	// Prefix hash: first min(4, N) messages
	prefixCount := 4
	if cs.MsgCount < prefixCount {
		prefixCount = cs.MsgCount
	}
	if prefixCount > 0 {
		var prefix strings.Builder
		for i := 0; i < prefixCount; i++ {
			fmt.Fprintf(&prefix, "%c", roleChar(msgs[i].Role))
			prefix.WriteString(msgs[i].Content[:min(80, len(msgs[i].Content))])
		}
		cs.PrefixHash = shortHash(prefix.String())
	}

	// Tail hash: last 4 messages
	tailCount := 4
	if cs.MsgCount < tailCount {
		tailCount = cs.MsgCount
	}
	if tailCount > 0 {
		var tail strings.Builder
		for i := cs.MsgCount - tailCount; i < cs.MsgCount; i++ {
			m := msgs[i]
			fmt.Fprintf(&tail, "%c", roleChar(m.Role))
			tailLen := len(m.Content)
			if tailLen > 80 {
				tailLen = 80
			}
			tail.WriteString(m.Content[:tailLen])
		}
		cs.TailHash = shortHash(tail.String())
	}

	return cs
}

// LastCacheShape returns the most recent cache-shape fingerprint (nil before
// the first turn). Thread-safe; intended for TCCAReport.
func (a *AgentRunner) LastCacheShape() *CacheShape {
	a.lastShapeMu.Lock()
	defer a.lastShapeMu.Unlock()
	return a.lastShape
}

// format renders the shape as a compact human+model-readable notice.
// Deprecated: no longer emitted as a Notice; kept for logging/debugging.
func (cs *CacheShape) format() string {
	var b strings.Builder
	b.WriteString("prefix-cache shape [" + cs.Kind + "]: ")
	fmt.Fprintf(&b, "msgs=%d roles=%s", cs.MsgCount, cs.Roles)
	if cs.SysHash != "" {
		fmt.Fprintf(&b, " sys=%s", cs.SysHash)
	}
	if cs.ToolsHash != "" {
		fmt.Fprintf(&b, " tools=%s", cs.ToolsHash)
	}
	if cs.PrefixHash != "" {
		fmt.Fprintf(&b, " prefix=%s", cs.PrefixHash)
	}
	if cs.TailHash != "" {
		fmt.Fprintf(&b, " tail=%s", cs.TailHash)
	}
	return b.String()
}

// roleChar maps a provider.Role to a single compact character for the shape.
func roleChar(r provider.Role) rune {
	switch r {
	case provider.RoleSystem:
		return 'S'
	case provider.RoleUser:
		return 'U'
	case provider.RoleAssistant:
		return 'A'
	case provider.RoleTool:
		return 'T'
	default:
		return '?'
	}
}

// shortHash returns the first 6 hex characters of the SHA-256 of s.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:6]
}
