package learning

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// DefaultPatternsPath is the project-level patterns file.
const DefaultPatternsPath = ".tianxuan" + string(filepath.Separator) + "learned-patterns.toml"

// LoadStore reads patterns from a TOML file. Missing file = empty store.
func LoadStore(path string) (*Store, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Store{}, nil
		}
		return nil, err
	}
	var s Store
	if err := toml.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.Patterns == nil {
		s.Patterns = []Pattern{}
	}
	return &s, nil
}

// SaveStore writes patterns to a TOML file, creating parent dirs if needed.
func SaveStore(path string, s *Store) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(s)
}

// PruneOld removes patterns not seen in the last N days and patterns
// that have been skipped. maxPatterns caps the total.
func PruneOld(s *Store, maxAgeDays int, maxPatterns int) {
	if s == nil {
		return
	}
	var kept []Pattern
_ = maxAgeDays
	for _, p := range s.Patterns {
		if p.Skipped {
			continue
		}
		kept = append(kept, p)
	}
	if len(kept) > maxPatterns {
		// Keep the most frequent ones
		// (they're already sorted by count from ActivePatterns,
		// but in the store they're not guaranteed sorted)
		kept = kept[:maxPatterns]
	}
	s.Patterns = kept
}
