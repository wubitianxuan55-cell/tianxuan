package memorycompiler

import (
	"time"
)

type CompressionReport struct {
	TraceID          string                  `json:"trace_id,omitempty"`
	Version          string                  `json:"version,omitempty"`
	CausalGraph      CausalGraphCompression  `json:"causal_graph,omitempty"`
	ExecutionTrace   ExecutionCompression    `json:"execution_trace,omitempty"`
	ControlGraph     ControlGraphCompression `json:"control_graph,omitempty"`
	MemoryGraph      MemoryGraphCompression  `json:"memory_graph,omitempty"`
	Alignment        CrossGraphAlignment     `json:"alignment,omitempty"`
	BiasCorrection   CompressionBiasReport   `json:"bias_correction,omitempty"`
	Dynamics         CausalSignalDynamics    `json:"dynamics,omitempty"`
	ObserverLoop     ObserverLoopReport      `json:"observer_loop,omitempty"`
	LayerCollapse    LayerCollapseReport     `json:"layer_collapse,omitempty"`
	CompressionRatio float64                 `json:"compression_ratio,omitempty"`
	CreatedAt        time.Time               `json:"created_at,omitempty"`
}

type CausalGraphCompression struct {
	TotalEdges      int            `json:"total_edges,omitempty"`
	RetainedEdges   int            `json:"retained_edges,omitempty"`
	DroppedEdges    int            `json:"dropped_edges,omitempty"`
	RelationCounts  map[string]int `json:"relation_counts,omitempty"`
	PrimaryCauses   []string       `json:"primary_causes,omitempty"`
	AnchorEdges     []CausalEdge   `json:"anchor_edges,omitempty"`
	LongTailEdges   []CausalEdge   `json:"long_tail_edges,omitempty"`
	LongTailSignals []string       `json:"long_tail_signals,omitempty"`
}

type ExecutionCompression struct {
	Outcome     string   `json:"outcome,omitempty"`
	Strategy    string   `json:"strategy,omitempty"`
	StepCount   int      `json:"step_count,omitempty"`
	ToolCalls   int      `json:"tool_calls,omitempty"`
	ToolErrors  int      `json:"tool_errors,omitempty"`
	KeyFindings []string `json:"key_findings,omitempty"`
	CostBand    string   `json:"cost_band,omitempty"`
	LatencyBand string   `json:"latency_band,omitempty"`
}

type ControlGraphCompression struct {
	Mode               string   `json:"mode,omitempty"`
	Controller         string   `json:"controller,omitempty"`
	ReportsFolded      int      `json:"reports_folded,omitempty"`
	StabilityBand      string   `json:"stability_band,omitempty"`
	OscillationBand    string   `json:"oscillation_band,omitempty"`
	EquilibriumState   string   `json:"equilibrium_state,omitempty"`
	TopSignals         []string `json:"top_signals,omitempty"`
	EquilibriumActions []string `json:"equilibrium_actions,omitempty"`
}

type MemoryGraphCompression struct {
	NodesFolded    int            `json:"nodes_folded,omitempty"`
	EdgesFolded    int            `json:"edges_folded,omitempty"`
	QualityCounts  map[string]int `json:"quality_counts,omitempty"`
	RelationCounts map[string]int `json:"relation_counts,omitempty"`
	AnchorNodes    []string       `json:"anchor_nodes,omitempty"`
	ConflictCount  int            `json:"conflict_count,omitempty"`
	NoiseCount     int            `json:"noise_count,omitempty"`
	TruthLockDecay []string       `json:"truth_lock_decay,omitempty"`
}

type CrossGraphAlignment struct {
	Status              string   `json:"status,omitempty"`
	AbstractionLevel    string   `json:"abstraction_level,omitempty"`
	SharedRelations     []string `json:"shared_relations,omitempty"`
	MissingFromMemory   []string `json:"missing_from_memory,omitempty"`
	MissingFromCausal   []string `json:"missing_from_causal,omitempty"`
	RawCouplingStrength float64  `json:"raw_coupling_strength,omitempty"`
	CouplingStrength    float64  `json:"coupling_strength,omitempty"`
	IndependenceStatus  string   `json:"independence_status,omitempty"`
	CouplingCapped      bool     `json:"coupling_capped,omitempty"`
}

type CompressionBiasReport struct {
	AnchorBudget      int      `json:"anchor_budget,omitempty"`
	LongTailRetained  int      `json:"long_tail_retained,omitempty"`
	LongTailRelations []string `json:"long_tail_relations,omitempty"`
	TruthLocksDecayed int      `json:"truth_locks_decayed,omitempty"`
	AlignmentStatus   string   `json:"alignment_status,omitempty"`
}

type CausalSignalDynamics struct {
	HierarchyGradient float64  `json:"hierarchy_gradient,omitempty"`
	SignalEntropy     float64  `json:"signal_entropy,omitempty"`
	EntropyBand       string   `json:"entropy_band,omitempty"`
	AmplitudeBand     string   `json:"amplitude_band,omitempty"`
	AmplifiedSignals  []string `json:"amplified_signals,omitempty"`
	EntropySpikes     []string `json:"entropy_spikes,omitempty"`
	CouplingStrength  float64  `json:"coupling_strength,omitempty"`
	Independence      string   `json:"independence,omitempty"`
	OverRegularized   bool     `json:"over_regularized,omitempty"`
}

type CausalEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

type ObserverLoopReport struct {
	Timeline             string                  `json:"timeline,omitempty"`
	ReadOnlyProjection   bool                    `json:"read_only_projection,omitempty"`
	CurrentTraceExcluded bool                    `json:"current_trace_excluded,omitempty"`
	LaggedSamples        int                     `json:"lagged_samples,omitempty"`
	LagWindow            AdaptiveLagWindow       `json:"lag_window,omitempty"`
	ShadowObserver       ShadowObserverReport    `json:"shadow_observer,omitempty"`
	TemporalSync         TemporalSyncReport      `json:"temporal_sync,omitempty"`
	SignalBacklog        PredictiveSignalBacklog `json:"signal_backlog,omitempty"`
	AdvisoryBridge       PredictionActionBridge  `json:"advisory_bridge,omitempty"`
	PredictionBias       PredictionBiasGuard     `json:"prediction_bias,omitempty"`
	TemporalVariance     TemporalVarianceReport  `json:"temporal_variance,omitempty"`
	LongTailSafety       LongTailSafetyReport    `json:"long_tail_safety,omitempty"`
	FeedbackEligible     bool                    `json:"feedback_eligible,omitempty"`
	FeedbackSignals      []string                `json:"feedback_signals,omitempty"`
	Damping              GlobalDampingEnvelope   `json:"damping,omitempty"`
}

type AdaptiveLagWindow struct {
	Size            int    `json:"size,omitempty"`
	Basis           string `json:"basis,omitempty"`
	StabilityBand   string `json:"stability_band,omitempty"`
	OscillationBand string `json:"oscillation_band,omitempty"`
}

type ShadowObserverReport struct {
	Mode                         string   `json:"mode,omitempty"`
	CurrentTraceObserved         bool     `json:"current_trace_observed,omitempty"`
	AffectsExecution             bool     `json:"affects_execution"`
	PredictedOscillationIndex    float64  `json:"predicted_oscillation_index,omitempty"`
	PredictionHorizon            int      `json:"prediction_horizon,omitempty"`
	WarningLevel                 string   `json:"warning_level,omitempty"`
	ObservationOnlySignals       []string `json:"observation_only_signals,omitempty"`
	FeedbackSignalsSuppressed    bool     `json:"feedback_signals_suppressed,omitempty"`
	ExecutionInfluenceSuppressed bool     `json:"execution_influence_suppressed"`
}

type TemporalSyncReport struct {
	Clock            string  `json:"clock,omitempty"`
	LagWindow        int     `json:"lag_window,omitempty"`
	DampingWindow    int     `json:"damping_window,omitempty"`
	NormalizedWindow int     `json:"normalized_window,omitempty"`
	DesyncIndex      float64 `json:"desync_index,omitempty"`
	Status           string  `json:"status,omitempty"`
}

type PredictiveSignalBacklog struct {
	Mode           string   `json:"mode,omitempty"`
	MaxSignals     int      `json:"max_signals,omitempty"`
	PendingSignals []string `json:"pending_signals,omitempty"`
	StaleSignals   []string `json:"stale_signals,omitempty"`
	PendingCount   int      `json:"pending_count,omitempty"`
	StaleCount     int      `json:"stale_count,omitempty"`
}

type PredictionActionBridge struct {
	Mode                      string   `json:"mode,omitempty"`
	AdvisoryEligible          bool     `json:"advisory_eligible,omitempty"`
	AdvisorySignals           []string `json:"advisory_signals,omitempty"`
	MaxAdvisories             int      `json:"max_advisories,omitempty"`
	RequiresExplicitPromotion bool     `json:"requires_explicit_promotion"`
	AffectsExecution          bool     `json:"affects_execution"`
	FeedbackBypassBlocked     bool     `json:"feedback_bypass_blocked"`
	BacklogResolved           bool     `json:"backlog_resolved,omitempty"`
}

type PredictionBiasGuard struct {
	Mode                       string   `json:"mode,omitempty"`
	CounterfactualChecks       []string `json:"counterfactual_checks,omitempty"`
	DriftRisk                  string   `json:"drift_risk,omitempty"`
	PlanningDriftBlocked       bool     `json:"planning_drift_blocked"`
	ExplorationPreserved       bool     `json:"exploration_preserved"`
	AdvisoryNeutralityEnforced bool     `json:"advisory_neutrality_enforced"`
}

type TemporalVarianceReport struct {
	Mode              string  `json:"mode,omitempty"`
	LogicalClock      string  `json:"logical_clock,omitempty"`
	PhysicalClock     string  `json:"physical_clock,omitempty"`
	PhysicalLatencyMs int64   `json:"physical_latency_ms,omitempty"`
	JitterIndex       float64 `json:"jitter_index,omitempty"`
	VarianceBand      string  `json:"variance_band,omitempty"`
	Normalization     string  `json:"normalization,omitempty"`
	VarianceVisible   bool    `json:"variance_visible"`
}

type LongTailSafetyReport struct {
	Mode              string   `json:"mode,omitempty"`
	RetentionFloor    int      `json:"retention_floor,omitempty"`
	RetainedSignals   []string `json:"retained_signals,omitempty"`
	DecayedSignals    []string `json:"decayed_signals,omitempty"`
	ProtectedSignals  []string `json:"protected_signals,omitempty"`
	RareSignalCount   int      `json:"rare_signal_count,omitempty"`
	LongTailPreserved bool     `json:"long_tail_preserved"`
}

type LayerCollapseReport struct {
	Mode                   string   `json:"mode,omitempty"`
	LayerCount             int      `json:"layer_count,omitempty"`
	ActiveLayers           []string `json:"active_layers,omitempty"`
	SemanticSaturationBand string   `json:"semantic_saturation_band,omitempty"`
	OverlapSignals         []string `json:"overlap_signals,omitempty"`
	OverConstraintRisk     string   `json:"over_constraint_risk,omitempty"`
	TemporalComplexity     string   `json:"temporal_complexity,omitempty"`
	SuggestedAbstractions  []string `json:"suggested_abstractions,omitempty"`
	RuntimeInfluence       bool     `json:"runtime_influence"`
	CacheSafe              bool     `json:"cache_safe"`
}

type GlobalDampingEnvelope struct {
	State             string   `json:"state,omitempty"`
	Factor            float64  `json:"factor,omitempty"`
	OscillationIndex  float64  `json:"oscillation_index,omitempty"`
	SuppressedSignals []string `json:"suppressed_signals,omitempty"`
}
