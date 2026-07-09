package session

import "strings"

// StripTransientBlocks removes leading transient blocks from user message
// content so the frontend displays clean user input without injected system
// directives. These blocks are prepended by withTurnPreferences, Compose, and
// memory injection at send time; they are NOT user content.
//
// Handles all 7 block types:
//   - reasoning-language / response-language (language hints)
//   - memory-update / session-facts (runtime memory state)
//   - background-jobs (completed background tasks)
//   - procedural-rules / episodic-memory (memory injection)
func StripTransientBlocks(content string) string {
	s := strings.TrimLeft(content, " \t\r\n")
	for {
		switch {
		case strings.HasPrefix(s, "<response-language>"):
			var ok bool
			s, ok = trimTransientBlock(s, "response-language")
			if !ok {
				return content
			}
		case strings.HasPrefix(s, "<reasoning-language>"):
			var ok bool
			s, ok = trimTransientBlock(s, "reasoning-language")
			if !ok {
				return content
			}
		case strings.HasPrefix(s, "<memory-update>"):
			var ok bool
			s, ok = trimTransientBlock(s, "memory-update")
			if !ok {
				return content
			}
		case strings.HasPrefix(s, "<session-facts>"):
			var ok bool
			s, ok = trimTransientBlock(s, "session-facts")
			if !ok {
				return content
			}
		case strings.HasPrefix(s, "<background-jobs>"):
			var ok bool
			s, ok = trimTransientBlock(s, "background-jobs")
			if !ok {
				return content
			}
		case strings.HasPrefix(s, "<procedural-rules>"):
			var ok bool
			s, ok = trimTransientBlock(s, "procedural-rules")
			if !ok {
				return content
			}
		case strings.HasPrefix(s, "<episodic-memory>"):
			var ok bool
			s, ok = trimTransientBlock(s, "episodic-memory")
			if !ok {
				return content
			}
		default:
			return s
		}
	}
}

func trimTransientBlock(content, tag string) (string, bool) {
	closeTag := "</" + tag + ">"
	i := strings.Index(content, closeTag)
	if i < 0 {
		return content, false
	}
	return strings.TrimLeft(content[i+len(closeTag):], " \t\r\n"), true
}
