package migration

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"tianxuan/internal/config"
	"tianxuan/internal/event"
)

// SessionImport records one legacy session source that contributed sessions.
type SessionImport struct {
	Source      string
	Destination string
	Count       int
}

// MemoryImport records one legacy memory source that contributed files.
type MemoryImport struct {
	Source      string
	Destination string
	Count       int
}

// Result summarizes an explicit migration rescue run.
type Result struct {
	Config         *config.MigrationResult
	ConfigErr      error
	MemoryImports  []MemoryImport
	MemoryErrs     []error
	SessionImports []SessionImport
	SessionErrs    []error
}

// Summary returns the final user-visible status.
func (r Result) Summary() string {
	importedSessions := 0
	for _, imp := range r.SessionImports {
		importedSessions += imp.Count
	}
	importedMemory := 0
	for _, imp := range r.MemoryImports {
		importedMemory += imp.Count
	}
	warnings := 0
	if r.ConfigErr != nil {
		warnings++
	}
	warnings += len(r.MemoryErrs)
	warnings += len(r.SessionErrs)
	switch {
	case warnings > 0:
		return fmt.Sprintf("migration rescue completed with %d warning(s): imported %d memory file(s) and %d past session(s)", warnings, importedMemory, importedSessions)
	case r.Config != nil || importedMemory > 0 || importedSessions > 0:
		parts := []string{}
		if r.Config != nil {
			parts = append(parts, "config/credentials")
		}
		if importedMemory > 0 {
			parts = append(parts, fmt.Sprintf("%d memory file(s)", importedMemory))
		}
		if importedSessions > 0 {
			parts = append(parts, fmt.Sprintf("%d past session(s)", importedSessions))
		}
		return "migration rescue complete: imported " + strings.Join(parts, " and ")
	default:
		return "migration rescue complete: no legacy data needed migration"
	}
}

// RunLegacyRescue retries the legacy migration path and emits progress notices.
func RunLegacyRescue(sink event.Sink) Result {
	emit := func(level event.Level, text string) {
		sink.Emit(event.Event{Kind: event.Notice, Level: level, Text: text})
	}
	result := Result{}
	emit(event.LevelInfo, "migration rescue: checking legacy config and credentials")
	migrated, err := config.MigrateLegacyIfNeeded()
	result.Config = migrated
	result.ConfigErr = err
	if err != nil {
		emit(event.LevelWarn, "migration rescue: config migration warning: "+err.Error())
	} else if migrated != nil {
		emit(event.LevelInfo, migrated.Notice())
	} else {
		emit(event.LevelInfo, "migration rescue: current config is already present or no legacy config was found")
	}
	result.Summary()
	return result
}

// copyFileIfMissing copies src to dst only if dst does not exist.
func copyFileIfMissing(src, dst string) (int, error) {
	in, err := os.Open(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		if os.IsExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(dst)
		return 0, err
	}
	return 1, nil
}

// CopyMissingTree recursively copies files from src to dst that don't exist at dst.
func CopyMissingTree(src, dst string) (int, error) {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return copyFileIfMissing(src, dst)
	}
	count := 0
	err = filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		n, err := copyFileIfMissing(path, target)
		count += n
		return err
	})
	return count, err
}
