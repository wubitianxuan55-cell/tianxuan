package config

import (
	"os"
	"path/filepath"
)

func userConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tianxuan", "config.toml")
}

// UserConfigPath is the user-global config file (~/.config/tianxuan/config.toml),
// or "" when the user config dir can't be resolved.
func UserConfigPath() string { return userConfigPath() }

// ArchiveDir is where compacted conversation history is archived for
// traceability (one timestamped .jsonl per compaction). Empty if the user config
// directory cannot be resolved, in which case archiving is skipped.
func ArchiveDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tianxuan", "archive")
}

// SessionDir is where chat sessions are persisted (one .jsonl per session).
// Used by `tianxuan chat --continue` / `--resume` to find the recent ones. Empty
// if the user config dir can't be resolved — sessions then aren't saved.
func SessionDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tianxuan", "sessions")
}

// WorkspaceSessionDir returns the workspace-scoped session directory under
// cwd/.tianxuan/sessions/. Sessions are isolated per workspace so switching
// projects shows only that workspace's history.
func WorkspaceSessionDir(cwd string) string {
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		} else {
			return SessionDir()
		}
	}
	return filepath.Join(cwd, ".tianxuan", "sessions")
}

// MemoryUserDir returns the tianxuan user config root (…/tianxuan), under which
// the user-global TIANXUAN.md and the per-project auto-memory store live. Empty
// when the user config dir can't be resolved, which disables user-scoped memory.
func MemoryUserDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tianxuan")
}

// ConventionDirs are the parent directories scanned for agent assets (skills,
// commands), in canonical-first order. .tianxuan is ours; .agents / .agent /
// .claude let users drop in assets authored for other agent tools without moving
// files. Shared so skills (internal/skill) and commands (CommandDirs) discover
// the same set. Note: hooks are NOT scanned across these — a .claude/settings.json
// uses a different hook schema that can't be parsed as ours, so hooks stay in
// .tianxuan/settings.json (see internal/hook).
var ConventionDirs = []string{".tianxuan", ".agents", ".agent", ".claude"}

// conventionSubdirsAsc joins sub under each ConventionDir of base, in ascending
// priority (reverse of ConventionDirs) so the canonical .tianxuan ends up the
// highest-priority entry — command.Load lets a later directory win on a clash.
func conventionSubdirsAsc(base, sub string) []string {
	out := make([]string, 0, len(ConventionDirs))
	for i := len(ConventionDirs) - 1; i >= 0; i-- {
		out = append(out, filepath.Join(base, ConventionDirs[i], sub))
	}
	return out
}

// CommandDirs returns the directories scanned for custom slash commands, lowest
// priority first, so a later (more specific) directory overrides an earlier one
// on a name clash. Order: home-dir convention dirs (~/.claude/commands … ~/.tianxuan/commands),
// the legacy XDG user dir (~/.config/tianxuan/commands), then the project's
// convention dirs (.claude/commands … .tianxuan/commands). Scanning the .claude /
// .agents / .agent dirs lets commands authored for other agent tools (same .md +
// frontmatter format) work here unchanged.
func CommandDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, conventionSubdirsAsc(home, "commands")...)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(dir, "tianxuan", "commands")) // legacy XDG user dir
	}
	dirs = append(dirs, conventionSubdirsAsc(".", "commands")...)
	return dirs
}

// SourcePath returns the highest-priority config file that exists, or "" if none.
func SourcePath() string {
	if _, err := os.Stat("tianxuan.toml"); err == nil {
		return "tianxuan.toml"
	}
	if uc := userConfigPath(); uc != "" {
		if _, err := os.Stat(uc); err == nil {
			return uc
		}
	}
	return ""
}
