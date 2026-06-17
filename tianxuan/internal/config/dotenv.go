package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// loadDotEnv loads .env files into the process environment. Priority:
//   ./.env (project-local) wins over ~/.env (global).
// Each file unconditionally sets its keys — stale env vars do not block
// fresh .env values. This is deliberate: the user edits .env to fix keys,
// and a leftover env var from a previous run must not override that choice.
func loadDotEnv() {
	// Global ~/.env first (fills in defaults).
	if home, err := os.UserHomeDir(); err == nil {
		loadDotEnvFile(filepath.Join(home, ".env"))
	}
	// Project ./.env last (overrides global for this workspace).
	loadDotEnvFile(".env")
}

// loadDotEnvFile reads one .env file (if present) and sets every key it
// declares, overwriting any previously-set value. Lenient, zero-dependency
// parsing. This always-override behaviour means editing .env reliably fixes
// stale keys without needing to hunt down inherited env vars.
func loadDotEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" {
			continue
		}
		os.Setenv(key, val)
	}
}
