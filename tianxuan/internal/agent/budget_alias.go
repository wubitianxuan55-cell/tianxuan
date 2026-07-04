package agent

import "tianxuan/internal/agent/budget"

// Re-exported types and functions from the budget sub-package.
type BudgetGate = budget.Gate
type BudgetStatus = budget.Status
type ModelProfile = budget.Profile

const (
	BudgetOK    = budget.OK
	BudgetWarn  = budget.Warn
	BudgetBlock = budget.Block
)

var (
	NewBudgetGate        = budget.NewGate
	DefaultModelProfiles = budget.DefaultProfiles
	LookupModelProfile   = budget.Lookup
)

// ApplyModelProfile is kept in the agent package because it depends on
// CompactionConfig, which is part of the compaction split (Phase 3).
func ApplyModelProfile(comp *CompactionConfig, profile *ModelProfile) {
	if profile == nil {
		return
	}
	if profile.ContextWindow > 0 {
		comp.Window = profile.ContextWindow
	}
	if profile.SoftRatio > 0 {
		comp.Ratio = profile.SoftRatio
	}
}
