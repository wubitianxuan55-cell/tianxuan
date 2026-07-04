package agent

import "tianxuan/internal/agent/textutils"

// Re-exported helpers from the textutils sub-package.
var (
	truncateToolOutput     = textutils.TruncateToolOutput
	truncateToolOutputWith = textutils.TruncateToolOutputWith
	normalizeText          = textutils.Normalize
	firstLine              = textutils.FirstLine
	hasSignalKeyword       = textutils.HasSignalKeyword
)

func visibleWidth(s string) int   { return textutils.VisibleWidth(s) }
func streamedRows(s string, w int) int { return textutils.StreamedRows(s, w) }
