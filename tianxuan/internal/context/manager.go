package context

import (
	"tianxuan/internal/cache"
	"tianxuan/internal/provider"
)

// ContextManager is the TCCA kernel — the unified entry point for all four
// context layers. AgentRunner and Controller consume it; it never depends
// on them. It knows nothing about Provider, planning, or tool execution.
type ContextManager struct {
	identity *IdentityLayer
	runtime  *RuntimeLayer
	skill    *SkillLayer
	flow     *FlowLayer
	metrics  *CacheMetrics
}

// NewContextManager creates the context kernel.
func NewContextManager(
	identity *IdentityLayer,
	runtime *RuntimeLayer,
	skill *SkillLayer,
	flow *FlowLayer,
) *ContextManager {
	return &ContextManager{
		identity: identity,
		runtime:  runtime,
		skill:    skill,
		flow:     flow,
		metrics:  NewCacheMetrics(),
	}
}

// ProcessFirstTurn runs the first-turn classification pipeline.
func (cm *ContextManager) ProcessFirstTurn(input string) TurnContext {
	profile := cm.skill.Route(input)
	cm.runtime.SetPromptHint(profile.PromptHint)
	cm.runtime.Lock()
	cm.metrics.SetL3Version(cm.skill.CurrentVersion())

	return TurnContext{
		SystemPrompt: cm.AssemblePrompt(),
		Tools:        cm.identity.FilteredSchemas(profile.Tools),
		Temperature:  profile.Temperature,
		MaxSteps:     profile.MaxSteps,
		RetryLimit:   profile.RetryLimit,
		Profile:      profile,
	}
}

// ProcessTurn returns the current turn context (L2 already locked).
func (cm *ContextManager) ProcessTurn() TurnContext {
	profile := cm.skill.CurrentProfile()
	return TurnContext{
		SystemPrompt: cm.AssemblePrompt(),
		Tools:        cm.identity.FilteredSchemas(profile.Tools),
		Temperature:  profile.Temperature,
		MaxSteps:     profile.MaxSteps,
		RetryLimit:   profile.RetryLimit,
		Profile:      profile,
	}
}

// AssemblePrompt builds the complete prompt for the Provider.
func (cm *ContextManager) AssemblePrompt() []provider.Message {
	msgs := make([]provider.Message, 0, 1+1+cm.flow.Len())

	l1 := cm.identity.SystemPrompt()
	if l1 != "" {
		msgs = append(msgs, provider.Message{Role: provider.RoleSystem, Content: l1})
	}

	l2 := cm.runtime.SystemPrompt()
	if l2 != "" {
		msgs = append(msgs, provider.Message{Role: provider.RoleSystem, Content: l2})
	}

	msgs = append(msgs, cm.flow.Messages()...)

	// Update metrics
	cm.metrics.SetLayerSizes(len(l1), len(l2), cm.flow.Len())

	return msgs
}

// Fork creates a child ContextManager for a sub-agent.
// ForkIndependent: shares L1 + L2.project, isolated L2.session/L3/L4.
// ForkCollaborative: also inherits L2.session (workspace state + execution memory).
func (cm *ContextManager) Fork(mode ForkMode, taskPrompt string) *ContextManager {
	childRuntime := NewRuntimeLayer()
	cm.runtime.CopyProjectTo(childRuntime)

	if mode == ForkCollaborative {
		cm.runtime.CopySessionTo(childRuntime)
	}

	child := &ContextManager{
		identity: cm.identity.Fork(),
		runtime:  childRuntime,
		skill:    cache.NewSkillLayer(),
		flow:     NewFlowLayer(cm.flow.CompactPolicy()),
		metrics:  cm.metrics.NewChild(),
	}

	return child
}

// RecordOutcome feeds execution results back to the SkillLayer's Learner.
func (cm *ContextManager) RecordOutcome(kind TaskKind, success bool) {
	cm.skill.RecordOutcome(kind, success)
}

// OnFileEdited tracks a file edit in the RuntimeLayer's session state.
func (cm *ContextManager) OnFileEdited(path string) {
	cm.runtime.TrackEdit(path)
}

// RecordCompact records token savings from a compaction pass.
func (cm *ContextManager) RecordCompact(savedTokens int64, pricePerToken float64) {
	cm.metrics.RecordCompact(savedTokens, pricePerToken)
}

// RecordFork records token savings from fork cache inheritance.
func (cm *ContextManager) RecordFork(savedTokens int64, pricePerToken float64) {
	cm.metrics.RecordFork(savedTokens, pricePerToken)
}

// === Query methods ===

func (cm *ContextManager) Identity() *IdentityLayer   { return cm.identity }
func (cm *ContextManager) Runtime() *RuntimeLayer      { return cm.runtime }
func (cm *ContextManager) Skill() *SkillLayer          { return cm.skill }
func (cm *ContextManager) Flow() *FlowLayer            { return cm.flow }
func (cm *ContextManager) Metrics() CacheReport        { return cm.metrics.Report() }

func (cm *ContextManager) ActiveTools() []provider.ToolSchema {
	return cm.identity.FilteredSchemas(cm.skill.CurrentProfile().Tools)
}

// TurnContext is the assembled context for one agent turn.
type TurnContext struct {
	SystemPrompt []provider.Message
	Tools        []provider.ToolSchema
	Temperature  float64
	MaxSteps     int
	RetryLimit   int
	Profile      SkillProfile
}
