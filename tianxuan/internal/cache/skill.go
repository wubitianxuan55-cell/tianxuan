package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// SkillProfile is the L3 execution policy for a task kind and version.
// It defines which tools are active, what behavioural hint to inject,
// and the conditions for version promotion.
//
// V3.0: replaces GoalRouter's hardcoded tool sets with versioned profiles.
// VerificationPolicy defines the verification requirements for a task kind.
type VerificationPolicy struct {
	RequireTests    bool
	RequireBuild    bool
	AutoReview      bool
	RequireCitation bool
}

type SkillProfile struct {
	Kind              TaskKind
	Tools             []string
	PromptHint        string
	Temperature       float64
	MaxSteps          int
	CompactThreshold  float64
	RetryLimit        int
	Verification      VerificationPolicy
}

// SkillLayer is the L3 cache domain — intent classification with adaptive
// version promotion. It replaces GoalRouter's monolithic Route() with a
// two-stage process: classify → select profile version → optionally promote.
//
// Profiles are indexed by [TaskKind][version] starting at version 1.
// Consecutive failures trigger PromoteVersion() which moves to the next
// version (broader tool set, more aggressive strategy).
//
// V3.3: FailReason classification prevents false upgrades from environment
// errors. Manual lock lets users pin a profile version.
type SkillLayer struct {
	current    SkillProfile
	version    int
	lockedVer  int  // V3.3: 0 = unlocked; >0 = locked to this version
	lockedKind TaskKind
}

// FailReason categorises why a tool call failed, so the adaptation logic
// can distinguish between "need more tools" and "environment issue".
type FailReason int

const (
	FailUnknown    FailReason = iota // uncategorized (default)
	FailToolBug                       // tool returned an error (retryable, may need upgrade)
	FailLogicError                    // tool returned wrong result (retryable)
	FailPermission                    // permission denied (do NOT upgrade — env issue)
	FailNetwork                       // network / API error (do NOT upgrade)
	FailEnvironment                   // missing dependency / wrong version (do NOT upgrade)
)

// ClassifyFailure heuristically categorises a tool error message.
// V3.3: only logic/tool errors trigger version promotion; env issues don't.
func ClassifyFailure(errMsg string) FailReason {
	if errMsg == "" {
		return FailUnknown
	}
	lower := strings.ToLower(errMsg)

	// Environment / network errors → do NOT upgrade
	if containsAny(lower, "permission denied", "access denied", "forbidden", "unauthorized") {
		return FailPermission
	}
	if containsAny(lower, "connection refused", "timeout", "no such host", "network", "dns", "dial tcp", "eof") {
		return FailNetwork
	}
	if containsAny(lower, "not found", "no such file", "command not found", "cannot find", "missing") {
		return FailEnvironment
	}

	// Logic / tool errors → MAY trigger upgrade
	if containsAny(lower, "error", "failed", "panic", "invalid", "unexpected", "wrong") {
		return FailToolBug
	}

	return FailUnknown
}

func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// LockVersion pins the profile to a specific version, disabling auto
// upgrade/downgrade. Pass 0 to unlock. V3.3: user-controlled via /skill lock.
func (l *SkillLayer) LockVersion(kind TaskKind, version int) {
	l.lockedKind = kind
	l.lockedVer = version
}

// IsLocked reports whether the profile is manually locked.
func (l *SkillLayer) IsLocked() bool { return l.lockedVer > 0 }

// Profiles defines the versioned skill profiles for each task kind.
// Version 1 = conservative (fewer tools, lower risk).
// Version 2 = expanded (add shell tools).
// Version 3 = maximum (add subagent tools).
var Profiles = map[TaskKind]map[int]SkillProfile{
	KindFixBug: {
		1: {Kind: KindFixBug, Tools: merge(readTools, editTools, shellTools, metaTools), PromptHint: "Reproduce first. Batch all file reads and searches in one response. Read → isolate → fix → verify.", Temperature: 0.3, MaxSteps: 20, RetryLimit: 3, Verification: VerificationPolicy{RequireBuild: true}},
		2: {Kind: KindFixBug, Tools: merge(readTools, editTools, shellTools, metaTools), PromptHint: "Write a test to reproduce. Batch reads together. Fix → verify with tests.", Temperature: 0.3, MaxSteps: 30, RetryLimit: 5, Verification: VerificationPolicy{RequireTests: true, RequireBuild: true}},
		3: {Kind: KindFixBug, Tools: merge(readTools, editTools, shellTools, metaTools, subagentTools), PromptHint: "Decompose into parallel sub-tasks. Batch reads per sub-task.", Temperature: 0.3, MaxSteps: 50, RetryLimit: 5, Verification: VerificationPolicy{RequireTests: true, RequireBuild: true, AutoReview: true}},
	},
	KindWriteFeature: {
		1: {Kind: KindWriteFeature, Tools: merge(readTools, editTools, shellTools, metaTools), PromptHint: "Design first. Read all relevant files in one batch. Keep changes minimal.", Temperature: 0.5, MaxSteps: 20, RetryLimit: 3, Verification: VerificationPolicy{RequireBuild: true}},
		2: {Kind: KindWriteFeature, Tools: merge(readTools, editTools, shellTools, metaTools), PromptHint: "Implement and test. Batch reads and searches in one step per phase.", Temperature: 0.5, MaxSteps: 30, RetryLimit: 5, Verification: VerificationPolicy{RequireTests: true, RequireBuild: true}},
		3: {Kind: KindWriteFeature, Tools: merge(readTools, editTools, shellTools, metaTools, subagentTools), PromptHint: "Decompose into sub-tasks. Batch reads per sub-task.", Temperature: 0.5, MaxSteps: 60, RetryLimit: 5, Verification: VerificationPolicy{RequireTests: true, RequireBuild: true, AutoReview: true}},
	},
	KindReview: {
		1: {Kind: KindReview, Tools: merge(readTools, metaTools), PromptHint: "Read all changed files at once. Check correctness, security, tests. Do NOT edit.", Temperature: 0, MaxSteps: 5, Verification: VerificationPolicy{AutoReview: true}},
	},
	KindExplain: {
		1: {Kind: KindExplain, Tools: merge(readTools, metaTools), PromptHint: "Read relevant code in one batch. Explain with references. Do NOT edit.", Temperature: 0, MaxSteps: 5},
	},
	KindResearch: {
		1: {Kind: KindResearch, Tools: merge(readTools, metaTools), PromptHint: "Search broadly first. Batch web searches and reads together. Cite sources.", Temperature: 0.7, MaxSteps: 30, Verification: VerificationPolicy{RequireCitation: true}},
		2: {Kind: KindResearch, Tools: merge(readTools, metaTools, subagentTools), PromptHint: "Use sub-agents for parallel exploration. Batch reads per sub-task.", Temperature: 0.7, MaxSteps: 60, Verification: VerificationPolicy{RequireCitation: true}},
	},
	KindDefault: {
		1: {Kind: KindDefault, Tools: nil, PromptHint: "Batch independent tool calls in a single response.", Temperature: 0.5, MaxSteps: 20, RetryLimit: 3},
	},
	// V4.0: non-code task kinds
	KindDataAnalysis: {
		1: {Kind: KindDataAnalysis, Tools: merge(readTools, shellTools, metaTools), PromptHint: "Load then explore. Batch independent data reads together. Load → explore → transform.", Temperature: 0.3, MaxSteps: 25, RetryLimit: 3, Verification: VerificationPolicy{RequireCitation: true}},
		2: {Kind: KindDataAnalysis, Tools: merge(readTools, shellTools, metaTools, subagentTools), PromptHint: "Use sub-agents for parallel exploration. Batch reads per sub-task.", Temperature: 0.3, MaxSteps: 50, RetryLimit: 5, Verification: VerificationPolicy{RequireCitation: true, AutoReview: true}},
	},
	KindWriting: {
		1: {Kind: KindWriting, Tools: merge(readTools, editTools, metaTools), PromptHint: "Read references first. Batch research in one step. Draft → revise → polish.", Temperature: 0.7, MaxSteps: 15},
	},
	KindGeneral: {
		1: {Kind: KindGeneral, Tools: nil, PromptHint: "Gather context first. Batch independent tool calls together.", Temperature: 0.5, MaxSteps: 20, RetryLimit: 3},
	},
}

// NewSkillLayer creates a SkillLayer with static profiles (V5.0: Learner removed).
func NewSkillLayer() *SkillLayer {
	return &SkillLayer{version: 1}
}

// Route classifies the input and returns the appropriate SkillProfile.
// It selects version 1 by default; if the Learner has recorded a promoted
// version for this kind, that version is used instead.
func (l *SkillLayer) Route(input string) SkillProfile {
	kind := classifyIntent(input)

	// Resolve version: Learner may have promoted it.
	version := l.resolveVersion(kind)
	profile, ok := Profiles[kind][version]
	if !ok {
		// Fallback: highest available version.
		for v := version; v >= 1; v-- {
			if p, ok2 := Profiles[kind][v]; ok2 {
				profile = p
				break
			}
		}
		// Ultimate fallback: default.
		if profile.Kind == "" {
			profile = Profiles[KindDefault][1]
		}
	}

	// V5.0: Learner removed — no dynamic tool adjustment.
	l.current = profile
	l.version = version
	return profile
}

// resolveVersion returns the profile version to use (V5.0: always v1, Learner removed).
func (l *SkillLayer) resolveVersion(kind TaskKind) int {
	if l.IsLocked() && l.lockedKind == kind {
		return l.lockedVer
	}
	return 1
}

// PromoteVersion is a no-op in V5.0 (Learner removed).
func (l *SkillLayer) PromoteVersion() {}

// RecordOutcome is a no-op in V5.0 (Learner removed).
func (l *SkillLayer) RecordOutcome(kind TaskKind, success bool) {}

// RecordOutcomeWithReason is a no-op in V5.0 (Learner removed).
func (l *SkillLayer) RecordOutcomeWithReason(kind TaskKind, success bool, errMsg string) {}

// DemoteVersion is a no-op in V5.0 (Learner removed).
func (l *SkillLayer) DemoteVersion() {}

// CurrentProfile returns the currently active profile.
func (l *SkillLayer) CurrentProfile() SkillProfile { return l.current }

// CurrentVersion returns the current profile version.
func (l *SkillLayer) CurrentVersion() int { return l.version }

// maxVersion returns the highest available version for a kind.
func maxVersion(kind TaskKind) int {
	versions, ok := Profiles[kind]
	if !ok {
		return 1
	}
	max := 0
	for v := range versions {
		if v > max {
			max = v
		}
	}
	return max
}

// AutoGenerateProfiles fills in missing v2/v3 profiles by deriving them from
// v1. Call this once during init to reduce manual maintenance of 18+ profiles.
// Rules:
//   - v2 = v1 + shellTools + MaxSteps*1.5 + RetryLimit=max(3, v1+2)
//   - v3 = v2 + subagentTools + MaxSteps*2.5 + AutoReview=true
//   - Temperature and PromptHint are preserved from v1 (adaptive version
//     upgrades only need more tools and steps, not different behaviour).
func AutoGenerateProfiles() {
	for kind, versions := range Profiles {
		v1, ok := versions[1]
		if !ok {
			continue
		}

		// Generate v2 if missing
		if _, hasV2 := versions[2]; !hasV2 {
			v2 := v1
			v2.Kind = kind
			v2.Tools = mergeUnique(v1.Tools, shellTools)
			v2.MaxSteps = maxInt(v1.MaxSteps*3/2, v1.MaxSteps+5) // at least +5
			v2.RetryLimit = maxInt(v1.RetryLimit, 3)
			v2.Verification.RequireTests = v1.Verification.RequireTests || true
			v2.Verification.RequireBuild = v1.Verification.RequireBuild || true
			Profiles[kind][2] = v2
		}

		// Generate v3 if missing (based on v2 to include shell tools)
		if _, hasV3 := versions[3]; !hasV3 {
			v2, ok := Profiles[kind][2]
			if !ok {
				continue
			}
			v3 := v2
			v3.Kind = kind
			v3.Tools = mergeUnique(v2.Tools, subagentTools)
			v3.MaxSteps = maxInt(v2.MaxSteps*5/3, v2.MaxSteps+15)
			v3.RetryLimit = maxInt(v2.RetryLimit, 5)
			v3.Verification.AutoReview = true
			Profiles[kind][3] = v3
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// LoadCustomProfiles scans dir for JSON profile definition files and merges
// them into the global Profiles map. Files are named <kind>.json (e.g.
// fix_bug.json) and contain a map of version→SkillProfile.
//
// V3.4: user-defined profiles in .tianxuan/skills/ override or extend the
// built-in 18 profiles. A custom profile replaces the built-in version
// entirely; partial overrides are not supported (the file must define
// complete profiles for each version).
//
// Example file (fix_bug.json):
//
//	{
//	  "1": {"kind":"fix_bug","tools":["read_file","edit_file","bash"],
//	        "promptHint":"Fix with minimal changes","temperature":0.2,
//	        "maxSteps":15,"retryLimit":2},
//	  "2": {"kind":"fix_bug","tools":["read_file","edit_file","bash","task"],
//	        "promptHint":"Fix with sub-agents","temperature":0.2,
//	        "maxSteps":25,"retryLimit":4}
//	}
func LoadCustomProfiles(dir string) (int, error) {
	if dir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // no custom profiles is not an error
		}
		return 0, err
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		kind := TaskKind(strings.TrimSuffix(entry.Name(), ".json"))
		path := filepath.Join(dir, entry.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files
		}

		var versions map[int]SkillProfile
		if err := json.Unmarshal(data, &versions); err != nil {
			continue // skip invalid JSON
		}

		// Replace or extend the built-in profiles for this kind
		if _, exists := Profiles[kind]; !exists {
			Profiles[kind] = make(map[int]SkillProfile)
		}
		for v, p := range versions {
			p.Kind = kind // enforce kind from filename
			Profiles[kind][v] = p
		}
		loaded++
	}
	return loaded, nil
}

// classifyIntent is the core classification logic (extracted from GoalRouter.Route).
func classifyIntent(input string) TaskKind {
	lower := strings.ToLower(input)

	if matchAnyWord(lower,
		"fix", "bug", "repair", "crash",
		"panic", "exception",
		"defect", "patch", "debug",
		"issue", "error", "fail", "broken",
		"wrong", "incorrect", "typo", "not working",
	) {
		return KindFixBug
	}
	if matchAnyWord(lower,
		"add", "create", "feature", "develop", "build",
		"implement", "refactor", "rewrite",
		"update", "change", "modify", "new", "extend",
	) {
		return KindWriteFeature
	}
	if matchAnyWord(lower,
		"review", "audit", "code review", "pr review",
		"inspect", "check", "validate", "verify",
	) {
		return KindReview
	}
	if matchAnyWord(lower,
		"explain", "analyze", "how does", "what does",
		"describe",
		"how to", "why", "what is", "meaning",
		"document", "tell me about",
	) {
		return KindExplain
	}
	// V4.0: non-code task classification
	if matchAnyWord(lower,
		"csv", "excel", "spreadsheet", "chart", "plot",
		"statistics", "data analysis", "visualize data",
		"pandas", "dataframe", "sql", "query",
	) {
		return KindDataAnalysis
	}
	if matchAnyWord(lower,
		"write", "article", "blog", "essay", "report",
		"draft", "polish",
	) && !matchAnyWord(lower, "code", "program", "function", "script") {
		return KindWriting
	}
	if matchAnyWord(lower,
		"help", "assist", "suggest", "recommend", "idea",
		"brainstorm", "plan", "organize", "summarize",
	) {
		return KindGeneral
	}
	return KindDefault
}
