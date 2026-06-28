// Package context implements the TCCA (Tianxuan Context Cache Architecture)
// four-layer context kernel. It is the unified entry point that AgentRunner
// and Controller consume — they never touch cache/ types directly.
//
// V3.0 Phase 5: ContextManager wraps IdentityLayer (L1), RuntimeLayer (L2),
// SkillLayer (L3), and FlowLayer (L4) into a single orchestration surface.
package context

import (
	"strings"
	"sync"
	"time"

	"tianxuan/internal/provider"
)

// CompactPolicy defines when and how compaction triggers.
type CompactPolicy struct {
	Window     int     // context window in tokens (0 = disabled)
	Ratio      float64 // trigger ratio (default 0.8)
	TailTokens int     // verbatim recent-tail budget in tokens
	// V3.2: optional callback to score detail importance (0-1).
	// When nil, all details get neutral importance (0.5) and eviction is FIFO.
	ImportanceFunc func(detail string) float64
}

// DefaultCompactPolicy returns the standard compaction policy.
func DefaultCompactPolicy() CompactPolicy {
	return CompactPolicy{
		Ratio:      0.8,
		TailTokens: 16384,
	}
}

// FlowLayer is the L4 domain — the append-only conversation history with
// compaction detail rings. It decouples flow management from AgentRunner.
type FlowLayer struct {
	mu      sync.Mutex
	store   MessageStore // V4.0: abstracted message storage
	rings   *DetailRingBuffer
	compact CompactPolicy
}

// DetailEntry is a single detail block in the ring buffer.
type DetailEntry struct {
	Content    string
	Importance float64 // 0-1, higher = more important
	Timestamp  int64   // unix nano, for tie-breaking
}

// DetailRingBuffer stores expanded compaction details for backtrack injection.
// V3.2: eviction is importance-based — when full, the lowest-importance entry
// is dropped first. Within equal importance, oldest wins.
type DetailRingBuffer struct {
	entries []DetailEntry
	dir     string // .tianxuan/deep/
	max     int    // 5
}

// NewFlowLayer creates a FlowLayer with the given compaction policy.
func NewFlowLayer(compact CompactPolicy) *FlowLayer {
	return &FlowLayer{
		compact: compact,
		rings:   &DetailRingBuffer{max: 5},
		store:   NewMemoryStore(), // V4.0: default in-memory store
	}
}

// SetStore replaces the message store (V4.0). Must be called before any
// messages are added. Passing nil is a no-op.
func (l *FlowLayer) SetStore(s MessageStore) {
	if s != nil {
		l.store = s
	}
}

// Add appends a message to the conversation history.
func (l *FlowLayer) Add(msg provider.Message) {
	_ = l.store.Append(msg)
}

// Messages returns the current conversation history.
func (l *FlowLayer) Messages() []provider.Message {
	msgs, _ := l.store.Range(0, l.store.Len())
	if msgs == nil {
		return []provider.Message{}
	}
	return msgs
}

// ReplaceMessages replaces the entire message list (used after compaction).
func (l *FlowLayer) ReplaceMessages(msgs []provider.Message) {
	_ = l.store.Truncate(0)
	for _, m := range msgs {
		_ = l.store.Append(m)
	}
}

// Len returns the number of messages.
func (l *FlowLayer) Len() int {
	return l.store.Len()
}

// Store returns the underlying MessageStore (V4.0).
func (l *FlowLayer) Store() MessageStore { return l.store }

// SetDetailDir sets the detail ring persistence directory.
func (l *FlowLayer) SetDetailDir(dir string) {
	l.rings.dir = dir
}

// PushDetail adds a detail block to the ring buffer. Thread-safe.
// Importance is computed from CompactPolicy.ImportanceFunc when set;
// otherwise defaults to 0.5 (neutral).
// V3.2: ring capacity scales dynamically with session length.
func (l *FlowLayer) PushDetail(detail string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Dynamic sizing: more messages → more rings
	l.rings.max = l.dynamicMax()

	importance := 0.5
	if l.compact.ImportanceFunc != nil {
		importance = l.compact.ImportanceFunc(detail)
	}

	entry := DetailEntry{
		Content:    detail,
		Importance: importance,
		Timestamp:  nowNano(),
	}

	l.rings.entries = append(l.rings.entries, entry)
	l.rings.evict()
}

// dynamicMax returns the ring capacity based on current message count.
// Short sessions don't need many rings; long sessions benefit from more.
func (l *FlowLayer) dynamicMax() int {
	n := l.store.Len()
	switch {
	case n < 20:
		return 3
	case n < 50:
		return 5
	default:
		return 8
	}
}

// PushDetailWithImportance adds a detail with an explicit importance score.
func (l *FlowLayer) PushDetailWithImportance(detail string, importance float64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := DetailEntry{
		Content:    detail,
		Importance: importance,
		Timestamp:  nowNano(),
	}

	l.rings.entries = append(l.rings.entries, entry)
	l.rings.evict()
}

// evict removes the lowest-importance entry when over max. Caller holds l.mu.
// Tie-break: older timestamp goes first.
func (r *DetailRingBuffer) evict() {
	for len(r.entries) > r.max {
		worst := 0
		for i := 1; i < len(r.entries); i++ {
			if r.entries[i].Importance < r.entries[worst].Importance ||
				(r.entries[i].Importance == r.entries[worst].Importance &&
					r.entries[i].Timestamp < r.entries[worst].Timestamp) {
				worst = i
			}
		}
		r.entries = append(r.entries[:worst], r.entries[worst+1:]...)
	}
}

// RecentDetail searches the ring buffer for a subject match. Thread-safe.
func (l *FlowLayer) RecentDetail(subject string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.rings.entries {
		if containsIgnoreCase(e.Content, subject) {
			return e.Content
		}
	}
	return ""
}

// nowNano returns the current time in nanoseconds. Extracted for testability.
var nowNano = func() int64 {
	return timeNow().UnixNano()
}

// timeNow is a shim for testing (can be replaced in tests).
var timeNow = func() time.Time {
	return time.Now()
}

// DetailDir returns the detail ring persistence directory.
func (l *FlowLayer) DetailDir() string { return l.rings.dir }

// CompactPolicy returns the compaction policy.
func (l *FlowLayer) CompactPolicy() CompactPolicy { return l.compact }

// SetCompactPolicy updates the compaction policy at runtime.
func (l *FlowLayer) SetCompactPolicy(p CompactPolicy) { l.compact = p }

// Window returns the context window size (0 = disabled).
func (l *FlowLayer) Window() int { return l.compact.Window }

// Ratio returns the compaction trigger ratio.
func (l *FlowLayer) Ratio() float64 { return l.compact.Ratio }

// TailTokens returns the verbatim tail budget.
func (l *FlowLayer) TailTokens() int { return l.compact.TailTokens }

func containsIgnoreCase(s, sub string) bool {
	if len(sub) == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
