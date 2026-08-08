package config

import "testing"

// TestProviderConfigured verifies Configured tracks whether the api_key_env
// resolves to a non-empty value — the same key check Validate enforces at build
// time, so model pickers can filter on it.
func TestProviderConfigured(t *testing.T) {
	t.Setenv("TIANXUAN_TEST_KEY", "secret")
	t.Setenv("TIANXUAN_TEST_EMPTY", "")

	cases := []struct {
		name string
		p    ProviderEntry
		want bool
	}{
		{"key set", ProviderEntry{APIKeyEnv: "TIANXUAN_TEST_KEY"}, true},
		{"key env empty", ProviderEntry{APIKeyEnv: "TIANXUAN_TEST_EMPTY"}, false},
		{"key env unset", ProviderEntry{APIKeyEnv: "TIANXUAN_TEST_MISSING"}, false},
		{"no api_key_env", ProviderEntry{}, false},
		{"opencode without key", ProviderEntry{Kind: "opencode"}, true},
		{"xai without key", ProviderEntry{Kind: "xai"}, true},
	}
	for _, c := range cases {
		if got := c.p.Configured(); got != c.want {
			t.Errorf("%s: Configured() = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestValidateOpencodeWithoutKey pins the keyless-open door for Zen: its free
// chat/completions models work anonymously, so Validate must not block an
// opencode provider that has no api_key_env.
func TestValidateOpencodeWithoutKey(t *testing.T) {
	c := &Config{
		Providers: []ProviderEntry{
			{Name: "zen", Kind: "opencode", BaseURL: "https://opencode.ai/zen/v1", Model: "deepseek-v4-flash-free"},
		},
	}
	if err := c.Validate("zen"); err != nil {
		t.Fatalf("Validate(zen) = %v, want nil", err)
	}
}
