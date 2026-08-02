package config

import "testing"

// TestBashTimeoutSecondsDefaults: omitted config = 120s safety cap (aligned
// with Reasonix).
func TestBashTimeoutSecondsDefaults(t *testing.T) {
	var cfg Config
	if got := cfg.BashTimeoutSeconds(); got != 120 {
		t.Fatalf("default = %d, want 120", got)
	}
}

// TestBashTimeoutSecondsExplicitZero: explicit 0 means no tool-local cap.
func TestBashTimeoutSecondsExplicitZero(t *testing.T) {
	z := 0
	cfg := Config{}
	cfg.Tools.BashTimeoutSeconds = &z
	if got := cfg.BashTimeoutSeconds(); got != 0 {
		t.Fatalf("explicit 0 = %d, want 0", got)
	}
}

// TestBashTimeoutSecondsNegativeFallsBack: negative config is invalid and
// falls back to the default cap instead of disabling it.
func TestBashTimeoutSecondsNegativeFallsBack(t *testing.T) {
	n := -5
	cfg := Config{}
	cfg.Tools.BashTimeoutSeconds = &n
	if got := cfg.BashTimeoutSeconds(); got != 120 {
		t.Fatalf("negative = %d, want 120 default", got)
	}
}

// TestBashTimeoutSecondsCustom: a configured value is honored.
func TestBashTimeoutSecondsCustom(t *testing.T) {
	v := 150
	cfg := Config{}
	cfg.Tools.BashTimeoutSeconds = &v
	if got := cfg.BashTimeoutSeconds(); got != 150 {
		t.Fatalf("custom = %d, want 150", got)
	}
}

// TestToolsCompactDefaultsEnabled locks the reduced-toolset default (V6.0 P8):
// a fresh Default() must enable compact and ship a small core whitelist so the
// model's per-turn schema stays small. Reverting to "all tools" here silently
// inflates every request's prefix and slows DeepSeek turns.
func TestToolsCompactDefaultsEnabled(t *testing.T) {
	cfg := Default()
	if !cfg.Tools.Compact {
		t.Fatal("default tools.compact must be true (reduced toolset)")
	}
	if len(cfg.Tools.Enabled) == 0 {
		t.Fatal("default tools.enabled must be a non-empty core whitelist")
	}
	if len(cfg.Tools.Enabled) > 25 {
		t.Fatalf("default whitelist too large: %d tools (want <= 25)", len(cfg.Tools.Enabled))
	}
}
