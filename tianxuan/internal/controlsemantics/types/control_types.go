package types

// Layer identifies a control layer in the Memory v5 compiler hierarchy.
type Layer int

const (
	LayerEquilibrium Layer = iota
	LayerStability
	LayerExploration
	LayerMutation
	LayerControl
)

// SignalType identifies the kind of control signal.
type SignalType int

const (
	SignalWeight SignalType = iota
	SignalConstraint
	SignalAdvisory
	SignalDecision
)

// TypedSignal carries a control signal between layers.
type TypedSignal struct {
	Type      SignalType
	Target    string
	Payload   interface{}
	Rationale string
}

// NewSignal creates a TypedSignal.
func NewSignal(sigType SignalType, target interface{}, payload interface{}, rationale string) TypedSignal {
	targetStr := ""
	if t, ok := target.(string); ok {
		targetStr = t
	}
	return TypedSignal{Type: sigType, Target: targetStr, Payload: payload, Rationale: rationale}
}
