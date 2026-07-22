package agent

import (
	"fmt"
	"strings"
)

// ── SDD 蒸馏实现 — OpenSpec 方法论映射到 Hermes ──────────────────────
//
// 以下函数将 OpenSpec 的 SDD 核心概念蒸馏到 Hermes 规划者中：
//   Delta Specs  — parseStepDeltas, countDeltas
//   Proposal     — hasProposal, extractProposal
//   Coherence    — checkCoherence
//   Verify 三维  — formatExecutionFeedbackEnhanced

// ── Delta Specs ──────────────────────────────────────────────────────

// proposalHeaders lists structured headers that mark a proposal section
// before the <!--plan--> marker. Analysis/investigation text without
// one of these headers is NOT a proposal — it's just preamble reasoning.
var proposalHeaders = []string{"## Proposal", "## 提案", "**Why**", "**Why:**", "## Why"}

// parseStepDeltas extracts the Delta type (ADDED/MODIFIED/REMOVED) for each
// step in a plan. Only steps with an explicit **Delta** field are included.
// Old-format plans without the Delta field return an empty map — backward
// compatible, no breaking change.
func parseStepDeltas(plan string) map[string]string {
	out := make(map[string]string)
	if !strings.Contains(plan, "<!--plan-->") {
		return out
	}
	lines := strings.Split(plan, "\n")
	var currentStep string
	inPlan := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!--plan-->") {
			inPlan = true
			continue
		}
		if !inPlan {
			continue
		}
		if planLineRE(trimmed) {
			// Step header: extract title after the number
			currentStep = extractStepTitle(trimmed)
			continue
		}
		if currentStep != "" && strings.HasPrefix(trimmed, "- **Delta**") {
			delta := extractFieldValue(trimmed, "**Delta**")
			delta = strings.TrimSpace(delta)
			switch delta {
			case "ADDED", "MODIFIED", "REMOVED":
				out[currentStep] = delta
			}
		}
	}
	return out
}

// extractStepTitle returns the step title from a "步骤 N：title" or "Step N：title" line.
func extractStepTitle(trimmed string) string {
	for _, prefix := range []string{"步骤 ", "Step "} {
		after, ok := strings.CutPrefix(trimmed, prefix)
		if !ok {
			continue
		}
		if len(after) == 0 || after[0] < '0' || after[0] > '9' {
			continue
		}
		// Skip the digit and optional "：" / ":" / space.
		return strings.TrimLeft(after[1:], "：: \t")
	}
	return trimmed
}

// extractFieldValue returns the value after a **Key**： or **Key**: marker.
func extractFieldValue(line, key string) string {
	s := line
	// Strip the bullet prefix.
	if after, ok := strings.CutPrefix(s, "- "); ok {
		s = after
	}
	// Strip **Key**： or **Key**:
	for _, sep := range []string{key + "：", key + ":"} {
		if after, ok := strings.CutPrefix(s, sep); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

// countDeltas returns per-type counts (ADDED/MODIFIED/REMOVED) across all
// steps in a plan. Used for summary display in execution feedback.
func countDeltas(plan string) map[string]int {
	deltas := parseStepDeltas(plan)
	out := make(map[string]int)
	for _, d := range deltas {
		out[d]++
	}
	return out
}

// ── Proposal ─────────────────────────────────────────────────────────

// hasProposal checks whether a plan text contains a structured proposal
// section before the <!--plan--> marker. Detects these patterns:
//
//	## Proposal
//	## 提案
//	**Why**:
//
// Analysis/investigation text without a structured header is NOT a proposal.
func hasProposal(plan string) bool {
	before, _, found := strings.Cut(plan, "<!--plan-->")
	if !found {
		return false
	}
	before = strings.TrimSpace(before)
	if before == "" {
		return false
	}
	lower := strings.ToLower(before)
	for _, h := range proposalHeaders {
		if strings.Contains(lower, strings.ToLower(h)) {
			return true
		}
	}
	return false
}

// extractProposal returns the proposal text (everything before <!--plan-->
// marker, excluding the marker itself). Returns "" when no proposal
// section exists.
func extractProposal(plan string) string {
	before, _, found := strings.Cut(plan, "<!--plan-->")
	if !found {
		return ""
	}
	before = strings.TrimSpace(before)
	if !hasProposal(plan) {
		return ""
	}
	return before
}

// ── Coherence ────────────────────────────────────────────────────────

// checkCoherence compares the files in a TurnResult against the plan's
// expected file list. It returns warnings for:
//   - Files modified but not in the plan (unplanned side effects)
//   - Files in the plan but not in the result (skipped steps, partial failure)
//
// This implements OpenSpec's "coherence" dimension of the verify triad.
func checkCoherence(plan string, result *TurnResult) []string {
	if result == nil {
		return nil
	}
	planFiles := extractPlanFiles(plan)
	if len(planFiles) == 0 {
		return nil
	}

	resultFiles := make(map[string]bool)
	for _, f := range result.FilesCreated {
		resultFiles[f] = true
	}
	for _, f := range result.FilesModified {
		resultFiles[f] = true
	}

	var warnings []string
	// Files in result but not in plan → unplanned side effects.
	for f := range resultFiles {
		if !planFiles[f] {
			warnings = append(warnings, fmt.Sprintf(
				"⚠ coherence: %s was modified but is not in the plan", f))
		}
	}
	// Files in plan but not in result → skipped or missed.
	for f := range planFiles {
		if !resultFiles[f] {
			warnings = append(warnings, fmt.Sprintf(
				"⚠ coherence: %s is in the plan but was not touched", f))
		}
	}
	return warnings
}

// extractPlanFiles returns the set of file paths mentioned in a plan's
// File(s) fields. [NEW] prefix is stripped.
func extractPlanFiles(plan string) map[string]bool {
	out := make(map[string]bool)
	lines := strings.Split(plan, "\n")
	inPlan := false
	inFilesField := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "<!--plan-->") {
			inPlan = true
			continue
		}
		if !inPlan {
			continue
		}
		// File(s) field can span multiple lines (e.g. "internal/foo.go, internal/bar.go").
		if strings.HasPrefix(trimmed, "- **File(s)**") || strings.HasPrefix(trimmed, "- **Files**") {
			inFilesField = true
			rest := extractFieldValue(trimmed, "**File(s)**")
			if rest == "" {
				rest = extractFieldValue(trimmed, "**Files**")
			}
			for _, f := range splitFileList(rest) {
				out[stripNewPrefix(f)] = true
			}
			continue
		}
		// Multi-line file list continuation: lines starting with exactly one level of indent
		// and no bullet (e.g. "internal/bar.go" on the next line after File(s)).
		if inFilesField {
			if planLineRE(trimmed) || strings.HasPrefix(trimmed, "- **") {
				inFilesField = false
				continue
			}
			if trimmed == "" {
				inFilesField = false
				continue
			}
			for _, f := range splitFileList(trimmed) {
				out[stripNewPrefix(f)] = true
			}
		}
	}
	return out
}

// splitFileList splits a comma/space/colon separated list of file paths.
func splitFileList(s string) []string {
	var parts []string
	// Split on common separators: comma, Chinese comma, semicolon.
	for _, part := range splitAny(s, ',', '，', ';') {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func splitAny(s string, seps ...rune) []string {
	sepSet := make(map[rune]bool, len(seps))
	for _, r := range seps {
		sepSet[r] = true
	}
	var result []string
	start := 0
	for i, r := range s {
		if sepSet[r] {
			if i > start {
				result = append(result, strings.TrimSpace(s[start:i]))
			}
			start = i + len(string(r))
		}
	}
	if start < len(s) {
		result = append(result, strings.TrimSpace(s[start:]))
	}
	return result
}

// stripNewPrefix removes the [NEW] marker from a file path.
func stripNewPrefix(f string) string {
	for _, marker := range []string{"[NEW] ", "[NEW]", "[new] ", "[new]"} {
		if after, ok := strings.CutPrefix(f, marker); ok {
			return strings.TrimSpace(after)
		}
	}
	return f
}

// ── Enhanced feedback ────────────────────────────────────────────────

// formatExecutionFeedbackEnhanced produces an OpenSpec-style execution
// summary for injection into the planner's session. It wraps the legacy
// formatExecutionFeedback and adds:
//   - Delta categorization (ADDED/MODIFIED/REMOVED counts)
//   - Verify triad (completeness/correctness/coherence checks)
//   - Step-level execution details (what each step did, with files)
func formatExecutionFeedbackEnhanced(r *TurnResult, plan string) string {
	legacy := formatExecutionFeedback(r)

	deltaLines := buildDeltaLines(r, plan)
	triad := buildVerifyTriad(r, plan)
	stepSummary := buildStepSummary(r)

	var b strings.Builder
	b.WriteString(legacy)

	if len(deltaLines) > 0 {
		b.WriteString("\n- Delta: ")
		b.WriteString(strings.Join(deltaLines, ", "))
	}
	if triad != "" {
		b.WriteString("\n- Verify: ")
		b.WriteString(triad)
	}
	if stepSummary != "" {
		b.WriteString("\n- Steps:\n")
		b.WriteString(stepSummary)
	}

	return b.String()
}

// buildStepSummary produces a per-step execution summary from StepResults.
// Each line: ✅/❌ step title — key output.
func buildStepSummary(r *TurnResult) string {
	if r == nil || len(r.StepResults) == 0 {
		return ""
	}
	var lines []string
	for _, sr := range r.StepResults {
		icon := "✅"
		if sr.Status != "success" {
			icon = "❌"
		}
		line := icon + " " + sr.Step
		if sr.Result != "" {
			line += " — " + sr.Result
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// buildDeltaLines returns Delta category lines. When plan has Delta markers, use them.
// When plan is empty or old-format (no Delta markers), infer from file operations:
//
//	FilesCreated → ADDED, FilesModified → MODIFIED.
func buildDeltaLines(r *TurnResult, plan string) []string {
	counts := countDeltas(plan)
	added := counts["ADDED"]
	modified := counts["MODIFIED"]
	removed := counts["REMOVED"]

	// If plan provided no Delta info, infer from result file operations.
	if added == 0 && modified == 0 && removed == 0 {
		added = len(r.FilesCreated)
		modified = len(r.FilesModified)
	}

	var lines []string
	if added > 0 {
		lines = append(lines, fmt.Sprintf("ADDED %d", added))
	}
	if modified > 0 {
		lines = append(lines, fmt.Sprintf("MODIFIED %d", modified))
	}
	if removed > 0 {
		lines = append(lines, fmt.Sprintf("REMOVED %d", removed))
	}
	return lines
}

// buildVerifyTriad constructs the OpenSpec-style verify summary:
// completeness / correctness / coherence.
func buildVerifyTriad(r *TurnResult, plan string) string {
	var parts []string

	// Completeness: all steps done?
	if len(r.StepResults) > 0 {
		done := 0
		for _, sr := range r.StepResults {
			if sr.Status == "success" {
				done++
			}
		}
		parts = append(parts, fmt.Sprintf("completeness=%d/%d", done, len(r.StepResults)))
	}

	// Correctness: tests passed?
	if r.Success && len(r.Errors) == 0 {
		parts = append(parts, "correctness=pass")
	} else {
		parts = append(parts, fmt.Sprintf("correctness=issues(%d)", len(r.Errors)))
	}

	// Coherence: files match plan?
	cohWarnings := checkCoherence(plan, r)
	if len(cohWarnings) == 0 {
		parts = append(parts, "coherence=ok")
	} else {
		parts = append(parts, fmt.Sprintf("coherence=warn(%d)", len(cohWarnings)))
	}

	return strings.Join(parts, " ")
}

// planLineRE is a helper that checks whether a trimmed line is a plan step
// header, matching the same logic as isStepLine in hermes_confirm.go.
// Redeclared here to keep sdd.go self-contained.
func planLineRE(trimmed string) bool {
	for _, prefix := range []string{"步骤 ", "Step "} {
		if after, ok := strings.CutPrefix(trimmed, prefix); ok {
			if len(after) > 0 && after[0] >= '0' && after[0] <= '9' {
				return true
			}
		}
	}
	return false
}
