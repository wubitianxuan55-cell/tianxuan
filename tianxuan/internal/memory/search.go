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
	// previews stores memory name → one-line description for display.
	previews map[string]string
}

// SearchMatch is a single search result with a relevance score and preview.
type SearchMatch struct {
	Name    string // memory slug
	Score   int    // number of matching tokens (higher = more relevant)
	Preview string // one-line description for display (from frontmatter)
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
		entries:  make(map[string][]string),
		bodies:   make(map[string]string, len(memories)),
		previews: make(map[string]string, len(memories)),
	}

	for _, m := range memories {
		// Combine title, description, and body for indexing.
		text := strings.ToLower(m.Title + " " + m.Description + " " + m.Body)
		idx.bodies[m.Name] = text

		// Store preview: display title if present, otherwise name.
		preview := m.Title
		if preview == "" {
			preview = strings.ReplaceAll(m.Name, "-", " ")
		}
		if m.Description != "" {
			preview += " — " + m.Description
		}
		idx.previews[m.Name] = preview

		// Extract unique tokens (3+ char alphanumeric or CJK sequences).
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
	// Also try substring fallback: for CJK queries where individual tokens
	// may not match, fall through to substring search over all bodies.
	if len(scores) == 0 {
		for name, body := range idx.bodies {
			if strings.Contains(body, queryTokens[0]) {
				scores[name] = 1
			}
		}
	}

	if len(scores) == 0 {
		return nil
	}

	// Convert to sorted slice (highest score first, then alphabetically).
	matches := make([]SearchMatch, 0, len(scores))
	for name, score := range scores {
		matches = append(matches, SearchMatch{
			Name:    name,
			Score:   score,
			Preview: idx.previews[name],
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Name < matches[j].Name
	})

	return matches
}

// tokenize splits text into lowercase alphanumeric and CJK tokens.
// ASCII/European words are split on non-letter boundaries as before.
// CJK characters (Han, Hiragana, Katakana, Hangul) are emitted as
// individual tokens of len=1, which the token-length gate in Search
// drops — so we concatenate consecutive CJK chars into bigrams.
func tokenize(text string) []string {
	var tokens []string
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	// Collect consecutive CJK characters, then emit bigrams for indexing.
	var cjkBuf []rune
	flushCJK := func() {
		if len(cjkBuf) == 0 {
			return
		}
		// Emit each 2-gram of consecutive CJK chars for indexability.
		// Single CJK chars are too short for the len<3 gate, so we use
		// overlapping bigrams: "缓存前缀" → ["缓存", "存前", "前缀"].
		// We also emit the full sequence as one token for exact-quote matches.
		full := string(cjkBuf)
		tokens = append(tokens, full)
		for i := 0; i+1 < len(cjkBuf); i++ {
			tokens = append(tokens, string([]rune{cjkBuf[i], cjkBuf[i+1]}))
		}
		cjkBuf = cjkBuf[:0]
	}

	for _, r := range text {
		if isCJK(r) {
			flush()
			cjkBuf = append(cjkBuf, unicode.ToLower(r))
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			flushCJK()
			current.WriteRune(unicode.ToLower(r))
		} else {
			flushCJK()
			flush()
		}
	}
	flushCJK()
	flush()

	return tokens
}

// isCJK reports whether r is a CJK character (Han, Hiragana, Katakana, Hangul).
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}
