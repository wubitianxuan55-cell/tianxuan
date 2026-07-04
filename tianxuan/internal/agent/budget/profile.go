package budget

// Profile defines context-window parameters for a specific model.
type Profile struct {
	Model         string
	ContextWindow int
	SoftRatio     float64 // default 0.8
}

// DefaultProfiles returns the built-in model profiles.
// Values from DeepSeek official docs.
func DefaultProfiles() []Profile {
	return []Profile{
		{Model: "deepseek-v4-flash", ContextWindow: 128_000, SoftRatio: 0.8},
		{Model: "deepseek-v4-pro", ContextWindow: 1_000_000, SoftRatio: 0.8},
		{Model: "deepseek-v3", ContextWindow: 128_000, SoftRatio: 0.8},
		{Model: "deepseek-r1", ContextWindow: 128_000, SoftRatio: 0.8},
		{Model: "deepseek-chat", ContextWindow: 128_000, SoftRatio: 0.8},
		{Model: "deepseek-reasoner", ContextWindow: 128_000, SoftRatio: 0.8},
	}
}

// Lookup finds a profile by model name. Returns nil when not found.
func Lookup(model string, profiles []Profile) *Profile {
	for i := range profiles {
		if profiles[i].Model == model {
			return &profiles[i]
		}
	}
	return nil
}
