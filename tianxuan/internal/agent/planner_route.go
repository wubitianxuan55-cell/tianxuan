package agent

import (
	"strings"
	"unicode/utf8"
)

// ── Planner feature analysis — distilled from reasonix-src ──────────────
//
// DecidePlannerRoute applies deterministic precedence rules to decide whether
// a turn needs the planner or can go executor-only. Pure text analysis —
// no model calls, no I/O. Used only within Hermes to skip the planner for
// simple tasks; never changes system prompts (executor always uses
// HephaestusSystemPrompt).

// PlannerRoute is the decision: executor_only skips the planner, plan_and_execute
// runs the full plan → confirm → execute pipeline.
type PlannerRoute string

const (
	RouteExecOnly      PlannerRoute = "executor_only"
	RoutePlanAndExec   PlannerRoute = "plan_and_execute"
)

// PlannerDecision is the deterministic routing result for one turn.
type PlannerDecision struct {
	Route  PlannerRoute
	Reason string
}

// DecidePlannerRoute classifies a user turn.
func DecidePlannerRoute(input string) PlannerDecision {
	text := strings.TrimSpace(input)
	if text == "" {
		return execDecision("empty")
	}
	if strings.HasPrefix(text, "/") {
		return execDecision("slash_command")
	}
	if _, ok := shouldSkipPlanner(input); ok {
		return execDecision("bang_prefix")
	}

	lower := strings.ToLower(text)

	if isShortReply(lower) {
		return execDecision("short_reply")
	}
	if isConversational(lower) {
		return execDecision("conversation")
	}
	if isLowRiskQuestion(lower) {
		return execDecision("low_risk_question")
	}

	f := planFeatures(text, lower)

	if !f.work {
		return execDecision("no_work")
	}
	if f.highRisk {
		return planDecision("high_risk")
	}
	if f.multiFile || f.crossSurface {
		return planDecision("cross_surface")
	}
	if f.structured {
		return planDecision("structured")
	}
	if f.complex {
		return planDecision("complex")
	}
	if f.atomic {
		return execDecision("atomic_edit")
	}
	if f.guidance {
		return planDecision("guidance")
	}
	if f.readOnly && !f.ambiguous {
		return execDecision("read_only")
	}
	// Pure directive: short, clear single-operation commands.
	// "构建" / "更新文件" / "更新记忆" — no planning needed.
	// Cap at 40 runes: longer inputs likely describe multi-step tasks.
	if f.work && !f.ambiguous && !f.crossSurface && !f.structured &&
		utf8.RuneCountInString(text) <= 40 {
		return execDecision("directive")
	}
	if f.ambiguous {
		return planDecision("ambiguous_work")
	}
	if f.anchored {
		return planDecision("anchored_work")
	}
	if f.work {
		return planDecision("work_request")
	}
	return execDecision("default")
}

func execDecision(reason string) PlannerDecision {
	return PlannerDecision{Route: RouteExecOnly, Reason: reason}
}

func planDecision(reason string) PlannerDecision {
	return PlannerDecision{Route: RoutePlanAndExec, Reason: reason}
}
