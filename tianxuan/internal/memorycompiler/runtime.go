package memorycompiler

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	runtimecanary "tianxuan/internal/runtime/canary"
	runtimeresource "tianxuan/internal/runtime/resource"
	runtimerollback "tianxuan/internal/runtime/rollback"
	runtimesandbox "tianxuan/internal/runtime/sandbox"
)

const (
	stateFile          = "state.json"
	tracesFile         = "traces.jsonl"
	version            = "v5.9"
	explorationRatePercent    = 10
	minExplorationRatePercent = 3
	maxExplorationRatePercent = 12
	mutationMinEvalTrials     = 2
	mutationAcceptThreshold   = 0.60
	mutationRegressionMargin  = 0.05
	mutationFeedbackCooldown  = 30 * time.Minute
	strategyDecayK            = 10.0
	staleConfidenceThreshold  = 0.2
	snapshotEveryExecutions   = 3
)

var runtimeLocks sync.Map

// Runtime owns one project's Memory v5 state.
type Runtime struct {
	dir string
	mu  *sync.Mutex
}

// New returns a runtime backed by dir. A blank dir disables persistence.
func New(dir string) *Runtime {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	dir = filepath.Clean(dir)
	return &Runtime{dir: dir, mu: runtimeLockForDir(dir)}
}

func runtimeLockForDir(dir string) *sync.Mutex {
	actual, _ := runtimeLocks.LoadOrStore(filepath.Clean(dir), &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// PlannerIR is the memory-compiled execution plan IR.
type PlannerIR struct {
	Version             string        `json:"version"`
	Goal                string        `json:"goal"`
	SourceEvent         string        `json:"source_event"`
	RuntimeMode         string        `json:"runtime_mode"`
	Constraints         []Constraint  `json:"constraints"`
	StrategySelection   *StrategyPick `json:"strategy_selection"`
	AvailableStrategies []StrategyRef `json:"available_strategies"`
	MemoryReferences    []MemoryRef   `json:"memory_references"`
	ExecutionSteps      []Step        `json:"execution_steps"`
	RiskNotes           []string      `json:"risk_notes"`
}

type Constraint struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
}

type StrategyRef struct {
	ID          string  `json:"id"`
	SuccessRate float64 `json:"success_rate"`
	Samples     int     `json:"samples"`
	Score       float64 `json:"score,omitempty"`
	Reason      string  `json:"reason,omitempty"`
}

type StrategyPick struct {
	Selected        string             `json:"selected"`
	Reason          string             `json:"reason"`
	Score           float64            `json:"score"`
	Mode            string             `json:"mode"`
	ExplorationRate float64            `json:"exploration_rate"`
	Rejected        []RejectedStrategy `json:"rejected"`
}

type RejectedStrategy struct {
	ID     string  `json:"id"`
	Reason string  `json:"reason"`
	Score  float64 `json:"score"`
}

type MemoryRef struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Quality   string `json:"quality,omitempty"`
	Influence string `json:"influence,omitempty"`
}

type Step struct {
	ID     string `json:"id"`
	Action string `json:"action"`
}

type ExecutionTrace struct {
	ID                  string                    `json:"id"`
	IRVersion           string                    `json:"ir_version"`
	Goal                string                    `json:"goal"`
	Steps               []Step                    `json:"steps,omitempty"`
	Outcome             string                    `json:"outcome"`
	EfficiencyScore     float64                   `json:"efficiency_score"`
	MemoryEffectiveness float64                   `json:"memory_effectiveness"`
	StrategyUsed        []string                  `json:"strategy_used,omitempty"`
	MemoryUsed          []string                  `json:"memory_used,omitempty"`
	CausalEdges         []CausalEdge              `json:"causal_edges,omitempty"`
	SemanticDrift       []string                  `json:"semantic_drift,omitempty"`
	ControlMode         string                    `json:"control_mode,omitempty"`
	ControlGain         float64                   `json:"control_gain,omitempty"`
	ControlSignals      []string                  `json:"control_signals,omitempty"`
	ProductionHardening *ProductionHardeningTrace `json:"production_hardening,omitempty"`
	Compression         *CompressionReport        `json:"compression,omitempty"`
	Cost                CostMetrics               `json:"cost,omitempty"`
	ToolResults         []ToolRecord              `json:"tool_results,omitempty"`
	FailureReason       string                    `json:"failure_reason,omitempty"`
	StartedAt           time.Time                 `json:"started_at"`
	CompletedAt         time.Time                 `json:"completed_at"`
}

type ToolRecord struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name"`
	Args       string `json:"args,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	ReadOnly   bool   `json:"read_only"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

type CostMetrics struct {
	EstimatedInputTokens      int   `json:"estimated_input_tokens,omitempty"`
	EstimatedCompiledTokens   int   `json:"estimated_compiled_tokens,omitempty"`
	EstimatedIROverheadTokens int   `json:"estimated_ir_overhead_tokens,omitempty"`
	LatencyMs                 int64 `json:"latency_ms,omitempty"`
	ToolCalls                 int   `json:"tool_calls,omitempty"`
	ToolErrors                int   `json:"tool_errors,omitempty"`
	TruncatedToolResults      int   `json:"truncated_tool_results,omitempty"`
}

type ProductionHardeningTrace struct {
	Sandbox              runtimesandbox.ExecutionSnapshot    `json:"sandbox,omitempty"`
	ResourceReservation  runtimeresource.Reservation         `json:"resource_reservation,omitempty"`
	ResourceDecision     runtimeresource.Decision            `json:"resource_decision,omitempty"`
	BudgetCoordinator    runtimeresource.CoordinatorSnapshot `json:"budget_coordinator,omitempty"`
	Canary               runtimecanary.Evaluation            `json:"canary,omitempty"`
	CanaryDiff           runtimecanary.BehaviorDiff          `json:"canary_diff,omitempty"`
	SnapshotID           string                              `json:"snapshot_id,omitempty"`
	RollbackDecision     runtimerollback.Decision            `json:"rollback_decision,omitempty"`
	EnforcementAuthority string                              `json:"enforcement_authority,omitempty"`
	Allowed              bool                                `json:"allowed"`
	BlockReasons         []string                            `json:"block_reasons,omitempty"`
}

type ControlPolicy struct {
	Version                string        `json:"version"`
	Mode                   string        `json:"mode"`
	Controller             string        `json:"controller"`
	ExplorationRatePercent int           `json:"exploration_rate_percent"`
	Gain                   float64       `json:"gain"`
	ConsensusScore         float64       `json:"consensus_score,omitempty"`
	Variance               float64       `json:"variance,omitempty"`
	EquilibriumState       string        `json:"equilibrium_state,omitempty"`
	EquilibriumActions     []string      `json:"equilibrium_actions,omitempty"`
	ControlGraphEntropy    float64       `json:"control_graph_entropy,omitempty"`
	SystemStabilityScore   float64       `json:"system_stability_score,omitempty"`
	ConvergenceVelocity    float64       `json:"convergence_velocity,omitempty"`
	OscillationIndex       float64       `json:"oscillation_index,omitempty"`
	MutationCooldown       time.Duration `json:"-"`
	MutationCooldownMs     int64         `json:"mutation_cooldown_ms"`
	SemanticShift          []string      `json:"semantic_shift,omitempty"`
	Reasons                []string      `json:"reasons,omitempty"`
}

type ControlReport struct {
	TraceID                string    `json:"trace_id,omitempty"`
	Mode                   string    `json:"mode"`
	Controller             string    `json:"controller"`
	ExplorationRatePercent int       `json:"exploration_rate_percent"`
	Gain                   float64   `json:"gain"`
	ConsensusScore         float64   `json:"consensus_score,omitempty"`
	Variance               float64   `json:"variance,omitempty"`
	EquilibriumState       string    `json:"equilibrium_state,omitempty"`
	EquilibriumActions     []string  `json:"equilibrium_actions,omitempty"`
	ControlGraphEntropy    float64   `json:"control_graph_entropy,omitempty"`
	SystemStabilityScore   float64   `json:"system_stability_score,omitempty"`
	ConvergenceVelocity    float64   `json:"convergence_velocity,omitempty"`
	OscillationIndex       float64   `json:"oscillation_index,omitempty"`
	MutationCooldownMs     int64     `json:"mutation_cooldown_ms"`
	SemanticShift          []string  `json:"semantic_shift,omitempty"`
	Reasons                []string  `json:"reasons,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
}

type MemoryQuality string

const (
	QualityHighSignal   MemoryQuality = "HIGH_SIGNAL"
	QualityMediumSignal MemoryQuality = "MEDIUM_SIGNAL"
	QualityNoise        MemoryQuality = "NOISE"
	QualityCorrupted    MemoryQuality = "CORRUPTED"
)

type MemoryNode struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Content     string      `json:"content"`
	Timestamp   time.Time   `json:"timestamp"`
	Confidence  float64     `json:"confidence"`
	Quality     MemoryQuality `json:"quality"`
	TruthLocked bool        `json:"truth_locked"`
	Constraint  *Constraint `json:"constraint,omitempty"`
}

type MemoryEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

type DecisionNode struct {
	ID              string    `json:"id"`
	Question        string    `json:"question"`
	SelectedOption  string    `json:"selected_option"`
	RejectedOptions []string  `json:"rejected_options,omitempty"`
	Reasoning       string    `json:"reasoning"`
	Timestamp       time.Time `json:"timestamp"`
}

type Strategy struct {
	ID              string    `json:"id"`
	Description     string    `json:"description,omitempty"`
	Successes       int       `json:"successes"`
	Failures        int       `json:"failures"`
	Score           float64   `json:"score,omitempty"`
	LastUsedAt      time.Time `json:"last_used_at"`
	Preconditions   []string  `json:"preconditions,omitempty"`
	ExecutionPlan   []Step    `json:"execution_plan,omitempty"`
}

type CompilerMutation struct {
	Target             string    `json:"target"`
	Change             string    `json:"change"`
	Reason             string    `json:"reason"`
	EvidenceTraceIDs   []string  `json:"evidence_trace_ids,omitempty"`
	Status             string    `json:"status,omitempty"`
	BaselineScore      float64   `json:"baseline_score,omitempty"`
	EvaluationScore    float64   `json:"evaluation_score,omitempty"`
	EvaluationReason   string    `json:"evaluation_reason,omitempty"`
	Applied            bool      `json:"applied"`
	CreatedAt          time.Time `json:"created_at,omitempty"`
	UpdatedAt          time.Time `json:"updated_at,omitempty"`
}

type MutationEvaluation struct {
	Target   string  `json:"target"`
	Change   string  `json:"change"`
	Reason   string  `json:"reason"`
	Decision string  `json:"decision"`
	Score    float64 `json:"score"`
	Baseline float64 `json:"baseline"`
	Trials   int     `json:"trials"`
}

type SystemLearning struct {
	TraceID              string    `json:"trace_id"`
	BadStrategies        []string  `json:"bad_strategies,omitempty"`
	GoodPatterns         []string  `json:"good_patterns,omitempty"`
	MemoryNoisePatterns  []string  `json:"memory_noise_patterns,omitempty"`
	CausalFindings       []string  `json:"causal_findings,omitempty"`
	CompilerImprovements []string  `json:"compiler_improvements,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

type state struct {
	Nodes              []MemoryNode          `json:"nodes,omitempty"`
	Edges              []MemoryEdge          `json:"edges,omitempty"`
	Decisions          []DecisionNode        `json:"decisions,omitempty"`
	Strategies         []Strategy            `json:"strategies,omitempty"`
	Mutations          []CompilerMutation    `json:"mutations,omitempty"`
	NoisyRefs          map[string]int        `json:"noisy_refs,omitempty"`
	Learning           SystemLearning        `json:"learning,omitempty"`
	ControlReports     []ControlReport       `json:"control_reports,omitempty"`
	CompressionReports []CompressionReport   `json:"compression_reports,omitempty"`
	Production         ProductionState       `json:"production,omitempty"`
	ExecutionState     ExecutionState        `json:"execution_state,omitempty"`
	DriftReports       []DriftReport         `json:"drift_reports,omitempty"`
	CreatedAt          time.Time             `json:"created_at,omitempty"`
	UpdatedAt          time.Time             `json:"updated_at,omitempty"`
}

type ExecutionState struct {
	GoalState         string       `json:"goal_state,omitempty"`
	CurrentPhase      string       `json:"current_phase,omitempty"`
	KnownFacts        []string     `json:"known_facts,omitempty"`
	ActiveConstraints []Constraint `json:"active_constraints,omitempty"`
	FailedStrategies  []string     `json:"failed_strategies,omitempty"`
	UpdatedAt         time.Time    `json:"updated_at,omitempty"`
}

type ProductionState struct {
	ExecutionCount          int                        `json:"execution_count,omitempty"`
	ExecutionsSinceSnapshot int                        `json:"executions_since_snapshot,omitempty"`
	LastSnapshotID          string                     `json:"last_snapshot_id,omitempty"`
	Rollbacks               []RollbackRecord           `json:"rollbacks,omitempty"`
	Budget                  runtimeresource.ResourceBudget `json:"budget,omitempty"`
}

type RollbackRecord struct {
	TraceID    string    `json:"trace_id"`
	SnapshotID string    `json:"snapshot_id"`
	Reasons    []string  `json:"reasons,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type DriftReport struct {
	TraceID            string    `json:"trace_id,omitempty"`
	OverusedStrategies []string  `json:"overused_strategies,omitempty"`
	StaleMemoryNodes   []string  `json:"stale_memory_nodes,omitempty"`
	ConflictingFacts   []string  `json:"conflicting_facts,omitempty"`
	CreatedAt          time.Time `json:"created_at,omitempty"`
}

type IRExplanation struct {
	DecisionSummary   string   `json:"decision_summary"`
	ConstraintMapping []string `json:"constraint_mapping"`
	MemoryInfluence   []string `json:"memory_influence"`
	StrategyReason    string   `json:"strategy_reason"`
}

// Compile is the main entry point: given a goal and source event, produce a
// PlannerIR that constrains and guides execution.
func (r *Runtime) Compile(goal, sourceEvent string) (*PlannerIR, IRExplanation, error) {
	if r == nil {
		return nil, IRExplanation{}, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	st := r.loadStateLocked()
	explanation := r.buildExplanation(st, goal, sourceEvent)
	ir := r.compileIR(st, goal, sourceEvent, explanation)
	return ir, explanation, nil
}

func (r *Runtime) compileIR(st state, goal, sourceEvent string, explanation IRExplanation) *PlannerIR {
	policy := r.resolveControlPolicy(st)
	strategies := r.selectStrategies(st, goal, policy)
	memoryRefs := r.selectMemoryReferences(st, policy)
	steps := r.planExecutionSteps(st, strategies, policy)

	return &PlannerIR{
		Version:             version,
		Goal:                goal,
		SourceEvent:         sourceEvent,
		RuntimeMode:         policy.Mode,
		Constraints:         r.buildConstraints(st, policy),
		StrategySelection:   strategies,
		AvailableStrategies: r.availableStrategies(st),
		MemoryReferences:    memoryRefs,
		ExecutionSteps:      steps,
		RiskNotes:           r.buildRiskNotes(st, policy),
	}
}

func (r *Runtime) resolveControlPolicy(st state) ControlPolicy {
	mode := "control"
	controller := "distributed-control-plane"
	explorationRate := explorationRatePercent
	gain := 1.0

	if len(st.ControlReports) > 0 {
		last := st.ControlReports[len(st.ControlReports)-1]
		mode = last.Mode
		controller = last.Controller
		explorationRate = last.ExplorationRatePercent
		gain = last.Gain
	}

	return ControlPolicy{
		Version:                version,
		Mode:                   mode,
		Controller:             controller,
		ExplorationRatePercent: explorationRate,
		Gain:                   gain,
		MutationCooldown:       mutationFeedbackCooldown,
	}
}

func (r *Runtime) loadStateLocked() state {
	var st state
	b, err := os.ReadFile(filepath.Join(r.dir, stateFile))
	if err != nil {
		return state{NoisyRefs: map[string]int{}, Production: normalizeProductionState(ProductionState{})}
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return state{NoisyRefs: map[string]int{}, Production: normalizeProductionState(ProductionState{})}
	}
	if st.NoisyRefs == nil {
		st.NoisyRefs = map[string]int{}
	}
	st.Production = normalizeProductionState(st.Production)
	return st
}

func (r *Runtime) buildExplanation(st state, goal, sourceEvent string) IRExplanation {
	return IRExplanation{
		DecisionSummary:   fmt.Sprintf("Use strategy %s for goal: %s", classifyStrategy(goal), summarizeGoal(sourceEvent)),
		ConstraintMapping: []string{},
		MemoryInfluence:   []string{},
		StrategyReason:    fmt.Sprintf("%.0f%% prior success after %d use(s)", r.strategySuccessRate(st, classifyStrategy(goal))*100, r.strategyUsageCount(st, classifyStrategy(goal))),
	}
}

func (r *Runtime) buildConstraints(st state, policy ControlPolicy) []Constraint {
	constraints := []Constraint{}
	for _, node := range st.Nodes {
		if node.Constraint != nil && (node.Quality == QualityHighSignal || node.Quality == QualityMediumSignal) {
			constraints = append(constraints, *node.Constraint)
		}
	}
	return constraints
}

func (r *Runtime) selectStrategies(st state, goal string, policy ControlPolicy) *StrategyPick {
	id := classifyStrategy(goal)
	pick := &StrategyPick{
		Selected:        id,
		Mode:            policy.Mode,
		ExplorationRate: float64(policy.ExplorationRatePercent),
		Rejected:        []RejectedStrategy{},
	}
	bestScore := -1.0
	for _, s := range st.Strategies {
		score := strategyScore(s)
		if score > bestScore {
			bestScore = score
		}
	}
	pick.Score = bestScore
	if pick.Score < 0 {
		pick.Score = 0.75
	}
	pick.Reason = fmt.Sprintf("%.0f%% prior success after %d use(s); %.2f novelty bonus; %.2f usage penalty",
		r.strategySuccessRate(st, id)*100,
		r.strategyUsageCount(st, id),
		0.15,
		0.03)
	return pick
}

func (r *Runtime) selectMemoryReferences(st state, policy ControlPolicy) []MemoryRef {
	refs := []MemoryRef{}
	for _, node := range st.Nodes {
		if node.Confidence >= 0.5 && node.Quality != QualityNoise && node.Quality != QualityCorrupted {
			refs = append(refs, MemoryRef{
				ID:        node.ID,
				Content:   node.Content,
				Quality:   string(node.Quality),
				Influence: "evidence",
			})
		}
	}
	if len(refs) > 10 {
		refs = refs[:10]
	}
	return refs
}

func (r *Runtime) planExecutionSteps(st state, pick *StrategyPick, policy ControlPolicy) []Step {
	for _, s := range st.Strategies {
		if s.ID == pick.Selected && len(s.ExecutionPlan) > 0 {
			return s.ExecutionPlan
		}
	}
	return []Step{
		{ID: "inspect", Action: "Inspect current state before acting."},
		{ID: "change", Action: "Make the smallest change that satisfies the task."},
		{ID: "check", Action: "Run focused validation and summarize evidence."},
	}
}

func (r *Runtime) availableStrategies(st state) []StrategyRef {
	refs := []StrategyRef{}
	for _, s := range st.Strategies {
		total := s.Successes + s.Failures
		rate := 0.5
		if total > 0 {
			rate = float64(s.Successes) / float64(total)
		}
		refs = append(refs, StrategyRef{
			ID:          s.ID,
			SuccessRate: rate,
			Samples:     total,
			Score:       strategyScore(s),
		})
	}
	return refs
}

func (r *Runtime) buildRiskNotes(st state, policy ControlPolicy) []string {
	notes := []string{}
	for _, report := range st.DriftReports {
		if len(report.OverusedStrategies) > 0 {
			for _, id := range report.OverusedStrategies {
				notes = append(notes, "drift control: reduce overused strategy "+id)
			}
		}
		if len(report.ConflictingFacts) > 0 {
			for _, conflict := range report.ConflictingFacts {
				notes = append(notes, "drift control: resolve memory conflict "+conflict)
			}
		}
	}
	return notes
}

func (r *Runtime) strategySuccessRate(st state, id string) float64 {
	for _, s := range st.Strategies {
		if s.ID == id {
			total := s.Successes + s.Failures
			if total == 0 {
				return 0.5
			}
			return float64(s.Successes) / float64(total)
		}
	}
	return 0.5
}

func (r *Runtime) strategyUsageCount(st state, id string) int {
	for _, s := range st.Strategies {
		if s.ID == id {
			return s.Successes + s.Failures
		}
	}
	return 0
}

func classifyStrategy(goal string) string {
	lower := strings.ToLower(goal)
	switch {
	case strings.Contains(lower, "review"):
		return "code-review"
	case strings.Contains(lower, "bug") || strings.Contains(lower, "fix"):
		return "bugfix-reproduce-first"
	case strings.Contains(lower, "frontend") || strings.Contains(lower, "ui"):
		return "frontend-visual-verify"
	case strings.Contains(lower, "goal") || strings.Contains(lower, "research"):
		return "long-horizon-autoresearch"
	default:
		return "general"
	}
}

func summarizeGoal(input string) string {
	input = strings.TrimSpace(input)
	input = strings.Join(strings.Fields(input), " ")
	if len([]rune(input)) > 180 {
		r := []rune(input)
		return string(r[:180]) + "..."
	}
	return input
}

func strategyScore(s Strategy) float64 {
	total := s.Successes + s.Failures
	if total == 0 {
		return 0.75
	}
	rate := float64(s.Successes) / float64(total)
	decay := math.Exp(-float64(total) / strategyDecayK)
	return rate*(1-decay) + 0.5*decay
}

func normalizeProductionState(ps ProductionState) ProductionState {
	if ps.Rollbacks == nil {
		ps.Rollbacks = []RollbackRecord{}
	}
	return ps
}

func traceID(t time.Time) string {
	return t.UTC().Format("20060102T150405.000000000")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func canonicalStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func limitStrings(in []string, n int) []string {
	if len(in) > n {
		return in[:n]
	}
	return in
}

func containsString(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
