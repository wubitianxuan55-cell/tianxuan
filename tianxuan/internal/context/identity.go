package context

import "tianxuan/internal/cache"

// === L1 Identity Layer ===
type IdentityLayer = cache.IdentityLayer

func NewIdentityLayer(systemPrompt string) *IdentityLayer {
	return cache.NewIdentityLayer(systemPrompt, nil)
}

// === L2 Runtime Layer ===
type RuntimeLayer = cache.RuntimeLayer
type ProjectState = cache.ProjectState
type SessionState = cache.SessionState
type ExecutionState = cache.ExecutionState
type TodoSnap = cache.TodoSnap

func NewRuntimeLayer() *RuntimeLayer { return cache.NewRuntimeLayer() }

// === L3 Skill Layer ===
type SkillLayer = cache.SkillLayer
type SkillProfile = cache.SkillProfile
type VerificationPolicy = cache.VerificationPolicy
type TaskKind = cache.TaskKind

const (
	KindFixBug      = cache.KindFixBug
	KindWriteFeature = cache.KindWriteFeature
	KindReview      = cache.KindReview
	KindExplain     = cache.KindExplain
	KindResearch    = cache.KindResearch
	KindDefault     = cache.KindDefault
)

// V5.0: Learner removed.
