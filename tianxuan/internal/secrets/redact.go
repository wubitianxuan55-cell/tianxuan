// Package secrets masks credential-like values in tool output and durable
// transcripts before they enter model context or on-disk storage.
// Ported from DeepSeek-Reasonix.
package secrets

import (
	"os"
	"regexp"
	"strings"
	"sync/atomic"

	"tianxuan/internal/provider"
)

var (
	// secretKeyNamePattern matches environment-variable / key names that are
	// likely to carry credentials.
	secretKeyNamePattern = regexp.MustCompile(`(?i)\b([A-Z0-9_.-]*(?:API[_-]?KEY|ACCESS[_-]?KEY|PRIVATE[_-]?KEY|SECRET|TOKEN|PASSWORD|PASSWD)[A-Z0-9_.-]*|[A-Z0-9_.-]+[_-]PWD[A-Z0-9_.-]*|AUTHORIZATION)\b`)
	keyValuePattern      = regexp.MustCompile(`(?i)\b([A-Z0-9_.-]*(?:API[_-]?KEY|ACCESS[_-]?KEY|PRIVATE[_-]?KEY|SECRET|TOKEN|PASSWORD|PASSWD)[A-Z0-9_.-]*|[A-Z0-9_.-]+[_-]PWD[A-Z0-9_.-]*|AUTHORIZATION)\b(['"]?\s*[:=]\s*['"]?)((?:Bearer|Basic|Digest|Negotiate|NTLM|Token|Bot|ApiKey)\s+)?(['"]?)([^'"\s,;]+)(['"]?)`)
	cookieHeaderPattern  = regexp.MustCompile(`(?i)\b((?:set-)?cookie)(\s*[:=]\s*)([^=;\s]+=[^;\s]*(?:;\s*[^=;\s]+(?:=[^;\s]*)?)*)`)
	cookiePairPattern    = regexp.MustCompile(`([^=;\s]+)=([^;\s]*)`)
	bearerTokenPattern   = regexp.MustCompile(`(?i)\bBearer\s+([A-Za-z0-9+/=_-]{16,})\b`)
	openAIKeyPattern     = regexp.MustCompile(`\b((?:sk|rk)-(?:proj-)?[A-Za-z0-9_-]{12,})\b`)
	githubTokenPattern   = regexp.MustCompile(`\b((?:gh[pous]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}))\b`)
	slackTokenPattern    = regexp.MustCompile(`\b(xox[bpars]-[A-Za-z0-9-]{16,})\b`)
	awsAccessKeyPattern  = regexp.MustCompile(`\b(AKIA[A-Z0-9]{16})\b`)
	jwtPattern           = regexp.MustCompile(`\b(eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)\b`)
)

const redactedValue = "[redacted]"

var (
	redactToolOutputEnabled      atomic.Bool
	filterSubprocessEnvEnabled   atomic.Bool
	protectSensitiveFilesEnabled atomic.Bool
)

func init() {
	redactToolOutputEnabled.Store(true)
}

// SetRedactToolOutput enables or disables masking of tool output.
func SetRedactToolOutput(enabled bool) { redactToolOutputEnabled.Store(enabled) }

// SetFilterSubprocessEnv enables or disables stripping credential-like variables.
func SetFilterSubprocessEnv(enabled bool) { filterSubprocessEnvEnabled.Store(enabled) }

// FilterSubprocessEnv reports whether credential-like variables are stripped.
func FilterSubprocessEnv() bool { return filterSubprocessEnvEnabled.Load() }

// SetProtectSensitiveFiles enables or disables the credential-path read denylist.
func SetProtectSensitiveFiles(enabled bool) { protectSensitiveFilesEnabled.Store(enabled) }

// ProtectSensitiveFiles reports whether the credential-path read denylist is active.
func ProtectSensitiveFiles() bool { return protectSensitiveFilesEnabled.Load() }

// EnvKeySensitive reports whether an environment variable name is likely to carry credentials.
func EnvKeySensitive(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	return secretKeyNamePattern.MatchString(key)
}

// FilterEnv removes sensitive KEY=value assignments from an environment vector.
func FilterEnv(env []string) []string {
	out := env[:0]
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if !ok || EnvKeySensitive(key) {
			continue
		}
		out = append(out, item)
	}
	return out
}

// ProcessEnv returns the environment for shell/tool subprocesses.
func ProcessEnv() []string {
	if !filterSubprocessEnvEnabled.Load() {
		return os.Environ()
	}
	return FilterEnv(os.Environ())
}

// RedactToolOutput masks credential-like values in live tool output.
func RedactToolOutput(s string) string {
	if !redactToolOutputEnabled.Load() {
		return s
	}
	return Redact(s)
}

// Redact masks credential-like values in text.
func Redact(s string) string {
	if s == "" {
		return s
	}
	s = keyValuePattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := keyValuePattern.FindStringSubmatch(match)
		if len(parts) != 7 {
			return redactedValue
		}
		key := parts[1]
		sep := parts[2]
		scheme := parts[3]
		quote := parts[4]
		value := parts[5]
		endQuote := parts[6]
		if strings.EqualFold(key, "authorization") {
			return key + sep + scheme + quote + redactedValue + endQuote
		}
		return key + sep + scheme + quote + mask(value) + endQuote
	})
	s = cookieHeaderPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := cookieHeaderPattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return redactedValue
		}
		return parts[1] + parts[2] + cookiePairPattern.ReplaceAllString(parts[3], "$1="+redactedValue)
	})
	s = bearerTokenPattern.ReplaceAllStringFunc(s, func(match string) string {
		token := strings.TrimSpace(strings.TrimPrefix(match, "Bearer"))
		if len(token) == len(match) {
			return "Bearer " + redactedValue
		}
		return "Bearer " + mask(token)
	})
	for _, rx := range []*regexp.Regexp{openAIKeyPattern, githubTokenPattern, slackTokenPattern, awsAccessKeyPattern, jwtPattern} {
		s = rx.ReplaceAllStringFunc(s, mask)
	}
	return s
}

func mask(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return redactedValue
	}
	if len(value) <= 12 {
		return redactedValue
	}
	head := 4
	tail := 4
	if strings.HasPrefix(value, "sk-") || strings.HasPrefix(value, "rk-") {
		head = 6
	}
	if len(value) <= head+tail {
		return redactedValue
	}
	return value[:head] + strings.Repeat("*", len(value)-head-tail) + value[len(value)-tail:]
}

// RedactMessage returns a storage-safe copy of m with textual secret surfaces masked.
func RedactMessage(m provider.Message) provider.Message {
	m.Content = Redact(m.Content)
	m.ReasoningContent = Redact(m.ReasoningContent)
	if len(m.ToolCalls) > 0 {
		calls := make([]provider.ToolCall, len(m.ToolCalls))
		copy(calls, m.ToolCalls)
		for i := range calls {
			calls[i].Arguments = Redact(calls[i].Arguments)
		}
		m.ToolCalls = calls
	}
	return m
}

// RedactMessages returns a redacted copy of msgs.
func RedactMessages(msgs []provider.Message) []provider.Message {
	out := make([]provider.Message, len(msgs))
	for i, m := range msgs {
		out[i] = RedactMessage(m)
	}
	return out
}
