package context

// ForkMode selects the sub-agent isolation strategy.
type ForkMode int

const (
	// ForkIndependent creates a fully independent sub-agent that shares only
	// L1 identity and L2 project state. Session state, skill, and flow are
	// all isolated. Used for research and exploration sub-tasks.
	ForkIndependent ForkMode = iota

// ForkCollaborative creates a sub-agent that inherits the parent's
// session state (recent edits, active module, execution memory) in
// addition to L1 and L2 project. Used for review and refactor sub-tasks
// that need awareness of the parent's current context.
ForkCollaborative
)
