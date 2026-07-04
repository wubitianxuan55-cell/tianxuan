// Package textutils provides tool output truncation, normalization, and
// terminal-width helpers used by the agent package.
package textutils

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// MaxToolOutputBytes caps a single tool result before it goes into the model's
// context window. 48 KiB keeps the overhead per call bounded while leaving room
// for meaningful output.
const MaxToolOutputBytes = 48 * 1024

const tailScanChars = 2048

const MaxToolResultLines = 320

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

// FirstLine returns the first line of s (up to, but not including, the first
// newline).
func FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TruncateToolOutput is the convenience wrapper that uses the default limits.
func TruncateToolOutput(s string) (string, string) {
	return TruncateToolOutputWith(s, MaxToolResultLines, MaxToolOutputBytes)
}

// TruncateToolOutputWith keeps the most important lines up to the limits.
func TruncateToolOutputWith(s string, maxLines, maxBytes int) (string, string) {
	s = Normalize(s)

	lines := strings.Split(s, "\n")
	originalLines := len(lines)
	originalBytes := len(s)

	if originalBytes <= maxBytes && originalLines <= maxLines {
		return s, ""
	}

	direction := "head+tail"
	if hasErrorInTail(s, tailScanChars) {
		direction = "tail"
	}
	selected := selectHygieneLines(lines, maxLines, direction)

	var out strings.Builder
	used := 0
	kept := 0
	for _, line := range selected {
		lineBytes := len(line) + 1
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

// Normalize strips ANSI escape sequences, normalises line endings, compresses
// blank-line runs and repeated consecutive lines.
func Normalize(s string) string {
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

// HasSignalKeyword reports whether the line contains a signal keyword.
func HasSignalKeyword(line string) bool {
	lower := strings.ToLower(line)
	for _, kw := range signalKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

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

	if direction == "tail" {
		// Error detected in tail: skip head, keep only tail + signal.
	} else {
		for i := 0; i < headCount && i < len(lines); i++ {
			indexes[i] = true
		}
	}
	for i := len(lines) - tailCount; i < len(lines); i++ {
		if i >= 0 {
			indexes[i] = true
		}
	}

	signalCount := 0
	for i := 0; i < len(lines) && len(indexes) < maxLines; i++ {
		if HasSignalKeyword(lines[i]) && !indexes[i] {
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

func snapToRuneBoundary(s string, lo, hi int) string {
	for lo > 0 && !utf8.RuneStart(s[lo]) {
		lo--
	}
	for hi < len(s) && !utf8.RuneStart(s[hi]) {
		hi++
	}
	return s[lo:hi]
}

// === Width helpers ===

// ansiSGR matches ANSI Select-Graphic-Rendition sequences.
var ansiSGR = regexp.MustCompile("\x1b\\[[0-9;]*m")

// VisibleWidth returns the column count of s after stripping ANSI SGR codes.
func VisibleWidth(s string) int {
	return runewidth.StringWidth(ansiSGR.ReplaceAllString(s, ""))
}

// StreamedRows counts how many rows the cursor has descended after raw text
// of length s was printed at the given terminal width.
func StreamedRows(s string, width int) int {
	if width <= 0 {
		width = 80
	}
	rows := 0
	for _, line := range strings.Split(s, "\n") {
		if w := VisibleWidth(line); w > 0 {
			rows += (w - 1) / width
		}
	}
	rows += strings.Count(s, "\n")
	return rows
}
