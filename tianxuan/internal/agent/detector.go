package agent

import "strings"

// FailureKind classifies a detected failure pattern.
type FailureKind string

const (
	KindNone               FailureKind = ""
	KindDependencyBlindspot FailureKind = "dependency-blindspot"
	KindToolMisuse         FailureKind = "tool-misuse"
	KindScopeCreep         FailureKind = "scope-creep"
)

// FailureReport describes a detected failure pattern and the corrective action.
type FailureReport struct {
	Kind    FailureKind
	Subject string // the tool or module involved
	Hint    string // corrective context to inject
}

// Detector analyzes failure patterns and produces corrective reports.
type Detector struct{}

// NewDetector returns a ready-to-use detector.
func NewDetector() *Detector { return &Detector{} }

// Analyze examines the storm signature and recent outcomes to classify the
// failure pattern. Returns nil when no actionable pattern is detected.
func (d *Detector) Analyze(stormSig string, outcomes []toolOutcome) *FailureReport {
	if stormSig == "" || len(outcomes) == 0 {
		return nil
	}

	// Collect error texts
	var errors []string
	for _, o := range outcomes {
		if o.errMsg != "" {
			errors = append(errors, o.errMsg)
		}
	}
	if len(errors) == 0 {
		return nil
	}
	combined := strings.Join(errors, " | ")

	// Pattern 1: dependency-blindspot — file/module not found errors
	if d.isDependencyBlindspot(combined) {
		return &FailureReport{
			Kind:    KindDependencyBlindspot,
			Subject: stormSig,
			Hint:    "Before modifying files in this area, use search_content to find all callers and imports. The error suggests a missing dependency or import chain.",
		}
	}

	// Pattern 2: tool-misuse — same tool failing repeatedly
	if d.isToolMisuse(stormSig, errors) {
		return &FailureReport{
			Kind:    KindToolMisuse,
			Subject: stormSig,
			Hint:    "The tool " + stormSig + " has failed repeatedly. Check its schema for required parameters, verify file paths exist before operating on them, and consider using a different tool for this operation.",
		}
	}

	return nil
}

func (d *Detector) isDependencyBlindspot(combined string) bool {
	indicators := []string{
		"file not found", "no such file", "cannot find",
		"undefined:", "is not defined", "unresolved",
		"import cycle", "missing dependency",
	}
	count := 0
	for _, kw := range indicators {
		if strings.Contains(strings.ToLower(combined), kw) {
			count++
		}
	}
	return count >= 1
}

func (d *Detector) isToolMisuse(sig string, errors []string) bool {
	if len(errors) < 2 {
		return false
	}
	// Detect consecutive identical errors — the model is stuck repeating
	// the same failing call.
	for i := 1; i < len(errors); i++ {
		if errors[i] == errors[i-1] {
			return true
		}
	}
	return false
}
