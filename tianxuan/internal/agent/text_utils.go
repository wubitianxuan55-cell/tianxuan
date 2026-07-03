package agent

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// maxToolOutputBytes caps a single tool result before it goes into the model's
// context window. 48 KiB keeps the overhead per call bounded while leaving room
// for meaningful output.
const maxToolOutputBytes = 48 * 1024
const tailScanChars = 2048 // scan last N chars for error patterns when truncation needed

const (
	maxToolResultLines = 320
)

var signalKeywords = []string{
	"error", "Error", "ERROR",
	"fatal", "Fatal", "FATAL",
	"panic", "PANIC",
	"exception", "Exception",
	"failed", "Failed",
	"timeout", "Timeout",
	"denied", "Denied",
	"cannot", "invalid",
}

// firstLine returns the first line of s (up to, but not including, the first
// newline).
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// truncateToolOutput is the convenience wrapper that uses the default limits.
func truncateToolOutput(s string) (string, string) {
	return truncateToolOutputWith(s, maxToolResultLines, maxToolOutputBytes)
}

// truncateToolOutputWith keeps the most important lines up to the limits.
// It strips ANSI escapes, compresses repeated lines, keeps head+tail+signal
// lines, and appends a truncation notice to the output.
func truncateToolOutputWith(s string, maxLines, maxBytes int) (string, string) {
	s = normalizeText(s)

	lines := strings.Split(s, "\n")
	originalLines := len(lines)
	originalBytes := len(s)

	if originalBytes <= maxBytes && originalLines <= maxLines {
		return s, ""
	}

	// If the tail contains error patterns, bias toward tail (diagnostics
	// are more valuable than startup noise); otherwise keep head+tail.
	direction := "head+tail"
	if hasErrorInTail(s, tailScanChars) {
		direction = "tail"
	}
	selected := selectHygieneLines(lines, maxLines, direction)

	var out strings.Builder
	used := 0
	kept := 0
	for _, line := range selected {
		lineBytes := len(line) + 1 // +1 for \n
		if used+lineBytes > maxBytes {
			if kept > 0 {
				break
			}
			if maxBytes < len(line) {
				line = snapToRuneBoundary(line, 0, maxBytes)
			}
		}
		if kept > 0 {
			out.WriteString("\n")
			used++
		}
		out.WriteString(line)
		used += len(line)
		kept++
	}

	omitted := originalBytes - used
	omittedLines := originalLines - kept
	notice := fmt.Sprintf("tool output truncated: %d of %d bytes, %d of %d lines elided",
		omitted, originalBytes, omittedLines, originalLines)

	out.WriteString(fmt.Sprintf(
		"\n\n[%d byte(s), %d line(s) elided above — use read_file with offset+limit, grep with narrower pattern, or bash with head/tail for details]",
		omitted, omittedLines))
	return out.String(), notice
}

// normalizeText strips ANSI escape sequences, normalises line endings, compresses
// blank-line runs and repeated consecutive lines.
func normalizeText(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' && s[j] != 'K' && s[j] != 'h' && s[j] != 'l' {
				if (s[j] >= '0' && s[j] <= '9') || s[j] == ';' || s[j] == '?' {
					j++
				} else {
					break
				}
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		out.WriteByte(s[i])
		i++
	}

	lines := strings.Split(strings.ReplaceAll(out.String(), "\r\n", "\n"), "\n")
	var result []string
	blankRun := 0
	prev := ""
	repeatCount := 0

	flushRepeat := func() {
		if repeatCount > 1 {
			result = append(result, fmt.Sprintf("[previous line repeated %d time(s)]", repeatCount-1))
		}
		repeatCount = 0
	}

	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			flushRepeat()
			blankRun++
			if blankRun <= 2 {
				result = append(result, "")
			}
			prev = ""
			continue
		}
		blankRun = 0
		if line == prev {
			repeatCount++
			continue
		}
		flushRepeat()
		result = append(result, line)
		prev = line
		repeatCount = 1
	}
	flushRepeat()
	return strings.Join(result, "\n")
}

// hasSignalKeyword reports whether the line contains a signal keyword.
func hasSignalKeyword(line string) bool {
	lower := strings.ToLower(line)
	for _, kw := range signalKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}



// hasErrorInTail scans the last n characters for error patterns.
func hasErrorInTail(s string, n int) bool {
	start := len(s) - n
	if start < 0 {
		start = 0
	}
	tail := strings.ToLower(s[start:])
	for _, kw := range signalKeywords {
		if strings.Contains(tail, kw) {
			return true
		}
	}
	return false
}
// selectHygieneLines picks head + tail + signal-bearing lines for truncation.
func selectHygieneLines(lines []string, maxLines int, direction string) []string {
	if len(lines) <= maxLines {
		return lines
	}

	indexes := make(map[int]bool)
	headCount := min(80, maxLines/4)
	tailCount := min(120, maxLines/3)
	if headCount < 1 {
		headCount = 1
	}
	if tailCount < 1 {
		tailCount = 1
	}

	for i := 0; i < headCount && i < len(lines); i++ {
		indexes[i] = true
	}
	for i := len(lines) - tailCount; i < len(lines); i++ {
		if i >= 0 {
			indexes[i] = true
		}
	}

	signalCount := 0
	if direction == "tail" {
		// Error detected in tail: drop head lines, keep only tail + signal.
		// headCount stays 0 (already set above).
	} else {
		for i := 0; i < headCount && i < len(lines); i++ {
			indexes[i] = true
		}
	}
	for i := 0; i < len(lines) && len(indexes) < maxLines; i++ {
		if hasSignalKeyword(lines[i]) && !indexes[i] {
			indexes[i] = true
			signalCount++
			if signalCount >= 48 {
				break
			}
		}
	}

	result := make([]string, 0, len(indexes))
	for i := 0; i < len(lines); i++ {
		if indexes[i] {
			result = append(result, lines[i])
		}
		if len(result) >= maxLines {
			break
		}
	}
	return result
}

// snapToRuneBoundary returns s[lo:hi] with the bounds nudged outward until
// both land on rune-start positions.
func snapToRuneBoundary(s string, lo, hi int) string {
	for lo > 0 && !utf8.RuneStart(s[lo]) {
		lo--
	}
	for hi < len(s) && !utf8.RuneStart(s[hi]) {
		hi++
	}
	return s[lo:hi]
}
