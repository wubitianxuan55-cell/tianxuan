package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"tianxuan/internal/tool"
)

// SessionFactPromoter is the interface for promoting session facts to permanent
// storage. The controller implements this; boot stamps it on the agent context.
type SessionFactPromoter interface {
	PromoteSessionFacts() (int, error)
}

type promoterKey struct{}

// WithPromoter stamps p onto ctx so promote_session_facts can read it.
func WithPromoter(ctx context.Context, p SessionFactPromoter) context.Context {
	return context.WithValue(ctx, promoterKey{}, p)
}

// PromoterFromContext returns the promoter, if any.
func PromoterFromContext(ctx context.Context) (SessionFactPromoter, bool) {
	p, ok := ctx.Value(promoterKey{}).(SessionFactPromoter)
	return p, ok && p != nil
}

// promoteSessionFactsTool lets the model finalize tentative session facts into
// permanent storage. It reads the promoter from context, following the same
// pattern as Queue and SessionSaver.
type promoteSessionFactsTool struct{}

// NewPromoteSessionFactsTool returns the `promote_session_facts` tool.
func NewPromoteSessionFactsTool() tool.Tool {
	return promoteSessionFactsTool{}
}

func (promoteSessionFactsTool) Name() string { return "promote_session_facts" }

func (promoteSessionFactsTool) Description() string {
	return "Promote all session-only memories (saved with remember(session=true)) to permanent storage. After promotion, they survive across sessions and load automatically like any other saved memory. Session facts are cleared after promotion."
}

func (promoteSessionFactsTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type": "object",
"properties": {},
"required": []
}`)
}

func (promoteSessionFactsTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	p, ok := PromoterFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("session promote unavailable in this context")
	}
	n, err := p.PromoteSessionFacts()
	if err != nil {
		return "", fmt.Errorf("promote failed: %w", err)
	}
	if n == 0 {
		return "No session facts to promote.", nil
	}
	return fmt.Sprintf("Promoted %d session fact(s) to permanent memory.", n), nil
}

func (promoteSessionFactsTool) ReadOnly() bool { return false }
