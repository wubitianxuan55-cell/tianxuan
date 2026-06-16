package context

// ProviderHint describes cache behaviour for different LLM providers.
// V4.0b: enables automatic strategy selection based on the active provider.
type ProviderHint struct {
	Name           string  // e.g. "deepseek", "openai", "anthropic"
	HasPrefixCache bool    // server-side prefix caching (DeepSeek: yes, Claude: no)
	CompactTrigger float64 // fraction of window that triggers compaction
	TailTokens     int     // verbatim tail budget in tokens
	Description    string  // human-readable note
}

// ProviderDefaults maps known provider names to their cache characteristics.
// Providers not listed here use the conservative defaults.
var ProviderDefaults = map[string]ProviderHint{
	"deepseek": {
		Name:           "deepseek",
		HasPrefixCache: true,
		CompactTrigger: 0.80, // aggressive: high cache hit rate allows later compaction
		TailTokens:     16384,
		Description:    "Server-side prefix cache, high hit rate → compact late",
	},
	"openai": {
		Name:           "openai",
		HasPrefixCache: true,
		CompactTrigger: 0.70, // automatic caching, mid-range trigger
		TailTokens:     12288,
		Description:    "Automatic caching (GPT-4o), moderate compaction trigger",
	},
	"anthropic": {
		Name:           "anthropic",
		HasPrefixCache: false,
		CompactTrigger: 0.60, // no server-side cache → compact earlier to save context
		TailTokens:     8192,
		Description:    "No prefix cache (Claude), compact early to conserve context window",
	},
}

// DefaultProviderHint returns the conservative fallback for unknown providers.
func DefaultProviderHint() ProviderHint {
	return ProviderHint{
		Name:           "unknown",
		HasPrefixCache: false,
		CompactTrigger: 0.65,
		TailTokens:     8192,
		Description:    "Unknown provider, conservative defaults",
	}
}

// LookupProvider returns the hint for a provider name (case-insensitive prefix match).
func LookupProvider(name string) ProviderHint {
	lower := toLower(name)
	for key, hint := range ProviderDefaults {
		if stringsContains(lower, key) {
			return hint
		}
	}
	return DefaultProviderHint()
}

// ApplyToPolicy modifies a CompactPolicy based on the provider hint.
func (h ProviderHint) ApplyToPolicy(p *CompactPolicy) {
	if p.Ratio <= 0 {
		p.Ratio = h.CompactTrigger
	}
	if p.TailTokens <= 0 {
		p.TailTokens = h.TailTokens
	}
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || findSubstring(s, substr) >= 0)
}

func findSubstring(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
