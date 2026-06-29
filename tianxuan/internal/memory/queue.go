package memory

import "context"

// Queue receives a one-line note about a memory change a tool just made, so the
// controller can fold it into the current turn — taking effect this session
// without touching the cache-stable system prefix. The remember/forget tools
// read it from their call context the same way background tools read the job
// manager.
type Queue interface{ QueueMemory(note string) }

type queueKey struct{}

// WithQueue stamps q onto ctx for the remember/forget tools to find.
func WithQueue(ctx context.Context, q Queue) context.Context {
	return context.WithValue(ctx, queueKey{}, q)
}

// QueueFromContext returns the memory queue the agent stamped, if any.
func QueueFromContext(ctx context.Context) (Queue, bool) {
	q, ok := ctx.Value(queueKey{}).(Queue)
	return q, ok && q != nil
}

// SessionSaver is an optional interface for saving a fact to session-only memory
// (not written to disk). The controller implements this and stamps it on the
// agent context; the remember tool uses it when session=true.
type SessionSaver interface {
	SaveSession(m Memory) string // returns a human-readable note
}

type sessionKey struct{}

// WithSessionSaver stamps s onto ctx.
func WithSessionSaver(ctx context.Context, s SessionSaver) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

// SessionSaverFromContext returns the session saver, if any.
func SessionSaverFromContext(ctx context.Context) (SessionSaver, bool) {
	s, ok := ctx.Value(sessionKey{}).(SessionSaver)
	return s, ok && s != nil
}
