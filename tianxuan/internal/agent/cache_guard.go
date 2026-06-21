package agent

import (
	"fmt"

	"tianxuan/internal/event"
)

// verifyPrefixAndShape captures the current PrefixShape and, on the first
// call, stores it as the session baseline. On each subsequent call it
// compares the live shape against the baseline — if L1 (system prompt) or
// tool schemas have drifted, it emits a Warning Notice instead of
// panicking (unlike the previous verifyPrefix). A drift means the
// DeepSeek prefix cache will miss on the next API call, but the session
// continues so the user can react.
func (a *AgentRunner) verifyPrefixAndShape() PrefixShape {
	shape := a.CaptureShape()

	if !a.prefixFingerprintSet {
		a.lastPrefixShape = shape
		a.prefixFingerprintSet = true
		return shape
	}

	prev := a.lastPrefixShape
	if prev.SystemHash != shape.SystemHash {
		a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
			Text: fmt.Sprintf("L1 identity changed! System hash: %s→%s — prefix cache will miss on next call",
				prev.SystemHash, shape.SystemHash)})
	}
	if prev.ToolsHash != shape.ToolsHash {
		a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
			Text: fmt.Sprintf("Tool schema changed! Tools hash: %s→%s — prefix cache will miss on next call",
				prev.ToolsHash, shape.ToolsHash)})
	}
	if prev.LogRewriteVersion != shape.LogRewriteVersion {
		// compact/prune happened — this is expected, just a Notice
		a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: fmt.Sprintf("Log rewrite: v%d→v%d — compaction/fold occurred, cache miss expected on next call",
				prev.LogRewriteVersion, shape.LogRewriteVersion)})
	}

	a.lastPrefixShape = shape
	return shape
}
