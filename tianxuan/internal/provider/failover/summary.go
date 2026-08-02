package failover

import "strings"

// Attempt records one candidate failure for the final summary.
type Attempt struct {
	Label string
	Err   error
}

// FallbackSummaryError is returned when every candidate failed. It carries
// per-attempt detail so the CLI can surface why each model was skipped
// (distilled from OpenClaw's FallbackSummaryError).
type FallbackSummaryError struct {
	Attempts []Attempt
}

// Error builds a compact summary naming every failed candidate.
func (e *FallbackSummaryError) Error() string {
	if e == nil {
		return "failover: all model candidates failed"
	}
	var sb strings.Builder
	sb.WriteString("failover: all model candidates failed:")
	for i, a := range e.Attempts {
		if i > 0 {
			sb.WriteString(";")
		}
		sb.WriteString(" ")
		sb.WriteString(a.Label)
		sb.WriteString(" (")
		if a.Err != nil {
			sb.WriteString(a.Err.Error())
		} else {
			sb.WriteString("unknown error")
		}
		sb.WriteString(")")
	}
	return sb.String()
}

// Unwrap exposes the last attempt's error for errors.Is/As callers that want
// the raw cause (e.g. an auth or parameter error that ended the chain).
func (e *FallbackSummaryError) Unwrap() error {
	if e == nil || len(e.Attempts) == 0 {
		return nil
	}
	return e.Attempts[len(e.Attempts)-1].Err
}

var _ error = (*FallbackSummaryError)(nil)
