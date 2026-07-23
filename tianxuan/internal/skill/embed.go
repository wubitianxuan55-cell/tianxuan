package skill

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// BundledSkills embeds the bundled skill files shipped with the binary.
// Extracted to ~/.tianxuan/skills/ on first run so Python scripts and
// data files are accessible as real files.
//
//go:generate cmd /c robocopy ..\..\..\..\.tianxuan\skills bundled /MIR /NJH /NJS /NFL
//
//go:embed all:bundled
var BundledSkills embed.FS

// EnsureBundled extracts the embedded skill files into the global skills
// directory (homeDir/.tianxuan/skills) when a skill is not already present
// there. A skill already in the target directory is left untouched so users
// can customise bundled skills and their changes survive upgrades.
func EnsureBundled(homeDir string) error {
	if homeDir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		homeDir = h
	}
	targetRoot := filepath.Join(homeDir, ".tianxuan", SkillsDirname)

	return fs.WalkDir(BundledSkills, "bundled", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Strip "bundled/" prefix to get the relative path inside the skills dir.
		rel := strings.TrimPrefix(path, "bundled")
		if rel == "" {
			return nil
		}
		rel = strings.TrimPrefix(rel, "/")
		rel = filepath.FromSlash(rel)

		target := filepath.Join(targetRoot, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		// Never overwrite an existing file — user customisations are preserved.
		if _, err := os.Stat(target); err == nil {
			return nil
		}

		data, err := BundledSkills.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
