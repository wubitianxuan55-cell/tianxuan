package memory

import (
	"sort"
	"strings"
	"unicode"
)

// SearchIndex is a simple in-memory inverted index over saved memories.
// Built once at load time and queried by the memory_search tool.
// Zero-allocation design: maps share backing memory with Store.
type SearchIndex struct {
	// entries maps token → list of memory names that contain the token.
	entries map[string][]string
	// bodies stores memory name → full text for ranking.
	bodies map[string]string
}

// SearchMatch is a single search result with a relevance score.
type SearchMatch struct {
	Name  string // memory slug
	Score int    // number of matching tokens (higher = more relevant)
}

// BuildSearchIndex reads all memory files in the store and builds an
// inverted index. Returns nil when the store is empty or unavailable.
func (s Store) BuildSearchIndex() *SearchIndex {
	if s.Dir == "" {
		return nil
	}
	memories := s.List()
	if len(memories) == 0 {
		return nil
	}

	idx := &SearchIndex{
		entries: make(map[string][]string),
		bodies:  make(map[string]string, len(memories)),
	}

	for _, m := range memories {
		// Combine title, description, and body for indexing.
		text := strings.ToLower(m.Title + " " + m.Description + " " + m.Body)
		idx.bodies[m.Name] = text

		// Extract unique tokens (3+ char alphanumeric sequences).
		seen := make(map[string]bool)
		for _, token := range tokenize(text) {
			if len(token) < 3 {
				continue
			}
			if seen[token] {
				continue
			}
			seen[token] = true
			idx.entries[token] = append(idx.entries[token], m.Name)
		}
	}

	return idx
}

// Search finds memories matching the query. Tokens are OR-matched
// and results are ranked by the number of matching tokens (descending).
// Returns nil when the index is empty or no matches found.
func (idx *SearchIndex) Search(query string) []SearchMatch {
	if idx == nil || len(idx.entries) == 0 {
		return nil
	}

	queryTokens := tokenize(strings.ToLower(query))
	if len(queryTokens) == 0 {
		return nil
	}

	// Count token matches per memory name.
	scores := make(map[string]int)
	for _, token := range queryTokens {
		if len(token) < 2 {
			continue
		}
		for _, name := range idx.entries[token] {
			scores[name]++
		}
	}

	if len(scores) == 0 {
		return nil
	}

	// Convert to sorted slice (highest score first, then alphabetically).
	matches := make([]SearchMatch, 0, len(scores))
	for name, score := range scores {
		matches = append(matches, SearchMatch{Name: name, Score: score})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Name < matches[j].Name
	})

	return matches
}

// tokenize splits text into lowercase alphanumeric tokens.
// CJK characters are kept as individual tokens for recall.
func tokenize(text string) []string {
	var tokens []string
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			current.WriteRune(unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()

	return tokens
}
