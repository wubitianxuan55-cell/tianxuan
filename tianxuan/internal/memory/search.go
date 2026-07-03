package memory

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// SearchIndex is an inverted index over saved memories with BM25 ranking.
// Built once at load time and queried by the memory_search tool.
// V10.18+: BM25 term weighting replaces pure token count; Docs (AGENTS.md etc.)
// are indexed alongside Store memories for unified search.
type SearchIndex struct {
	// entries maps token → list of (memory name, term frequency in that document).
	entries map[string][]tfEntry
	// docLen stores memory name → document length (token count).
	docLen map[string]int
	// totalDocs is the number of indexed documents.
	totalDocs int
	// avgDocLen is the average document length in tokens.
	avgDocLen float64
	// previews stores memory name → one-line description for display.
	previews map[string]string
	// kinds stores memory name → Kind (for filtering).
	kinds map[string]Kind

	// BM25 parameters (standard defaults).
	k1 float64 // term frequency saturation (default 1.2)
	b  float64 // length normalization (default 0.75)
}

type tfEntry struct {
	name string
	tf   int // term frequency in this document
}

// SearchMatch is a single search result with a BM25 relevance score and preview.
type SearchMatch struct {
	Name    string // memory slug
	Score   float64
	Preview string // one-line description for display (from frontmatter)
	Kind    Kind   // semantic / episodic / procedural
}

// BuildSearchIndex reads all memory files in the store and docs from the
// provided doc list, then builds an inverted index with BM25 statistics.
// Returns nil when there is nothing to index.
func (s Store) BuildSearchIndex(docs []Source) *SearchIndex {
	memories := s.List()
	if len(memories) == 0 && len(docs) == 0 {
		return nil
	}

	idx := &SearchIndex{
		entries:  make(map[string][]tfEntry),
		docLen:   make(map[string]int),
		previews: make(map[string]string),
		kinds:    make(map[string]Kind),
		k1:       1.2,
		b:        0.75,
	}

	// Index Store memories.
	for _, m := range memories {
		text := strings.ToLower(m.Title + " " + m.Description + " " + m.Body)
		idx.indexDoc(m.Name, text, m.Title, m.Description, m.Kind)
	}

	// Index Docs (AGENTS.md etc.) under a "doc:" prefix namespace.
	for _, d := range docs {
		name := "doc:" + d.Path
		text := strings.ToLower(d.Body)
		idx.indexDoc(name, text, filepathBase(d.Path), "", KindSemantic)
	}

	// Compute average document length.
	if idx.totalDocs > 0 {
		var total int
		for _, l := range idx.docLen {
			total += l
		}
		idx.avgDocLen = float64(total) / float64(idx.totalDocs)
	}

	return idx
}

// filepathBase returns the last element of a path, avoiding import of path/filepath.
func filepathBase(path string) string {
	path = strings.TrimRight(path, "/\\")
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}

// indexDoc adds one document (memory or doc) to the index.
func (idx *SearchIndex) indexDoc(name, text, title, desc string, kind Kind) {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return
	}
	idx.totalDocs++
	idx.docLen[name] = len(tokens)
	idx.kinds[name] = NormalizeKind(string(kind))

	// Preview.
	preview := title
	if preview == "" {
		preview = strings.ReplaceAll(name, "-", " ")
	}
	if desc != "" {
		preview += " — " + desc
	}
	idx.previews[name] = preview

	// Token frequency in this document.
	tf := make(map[string]int)
	for _, tok := range tokens {
		if len(tok) < 2 {
			continue
		}
		tf[tok]++
	}
	for tok, count := range tf {
		idx.entries[tok] = append(idx.entries[tok], tfEntry{name: name, tf: count})
	}
}

// Search finds memories matching the query with BM25 ranking.
// Returns results sorted by score descending. To filter by kind, use SearchByKind.
func (idx *SearchIndex) Search(query string) []SearchMatch {
	return idx.searchFiltered(query, "")
}

// SearchByKind finds memories of a specific kind matching the query.
// Empty kind means no filter (same as Search).
func (idx *SearchIndex) SearchByKind(query string, kind Kind) []SearchMatch {
	return idx.searchFiltered(query, kind)
}

func (idx *SearchIndex) searchFiltered(query string, kind Kind) []SearchMatch {
	if idx == nil || len(idx.entries) == 0 {
		return nil
	}

	queryTokens := tokenize(strings.ToLower(query))
	if len(queryTokens) == 0 {
		return nil
	}

	// BM25 scoring.
	scores := make(map[string]float64)
	for _, token := range queryTokens {
		if len(token) < 2 {
			continue
		}
		entries, ok := idx.entries[token]
		if !ok {
			continue
		}
		// IDF: log((N - n + 0.5) / (n + 0.5) + 1)
		n := len(entries)
		idf := math.Log((float64(idx.totalDocs)-float64(n)+0.5)/(float64(n)+0.5) + 1.0)

		for _, e := range entries {
			if kind != "" && idx.kinds[e.name] != kind {
				continue
			}
			docLen := float64(idx.docLen[e.name])
			tf := float64(e.tf)
			// BM25 term score: IDF * (tf * (k1+1)) / (tf + k1 * (1-b + b * docLen/avgLen))
			norm := 1 - idx.b + idx.b*docLen/idx.avgDocLen
			score := idf * (tf * (idx.k1 + 1)) / (tf + idx.k1*norm)
			scores[e.name] += score
		}
	}

	// Substring fallback: when BM25 finds nothing, try raw substring match.
	if len(scores) == 0 && len(queryTokens) > 0 {
		for name, preview := range idx.previews {
			if kind != "" && idx.kinds[name] != kind {
				continue
			}
			if strings.Contains(strings.ToLower(preview), queryTokens[0]) {
				scores[name] = 0.1
			}
		}
	}

	if len(scores) == 0 {
		return nil
	}

	// Sort by score descending, then alphabetically.
	matches := make([]SearchMatch, 0, len(scores))
	for name, score := range scores {
		matches = append(matches, SearchMatch{
			Name:    name,
			Score:   score,
			Preview: idx.previews[name],
			Kind:    idx.kinds[name],
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if math.Abs(matches[i].Score-matches[j].Score) > 0.001 {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Name < matches[j].Name
	})

	return matches
}

// --- Tokenization ---

// Common CJK stopwords that add noise to search indexes.
var cjkStopwords = map[string]bool{
	"的": true, "了": true, "是": true, "在": true, "和": true,
	"也": true, "就": true, "都": true, "而": true, "及": true,
	"与": true, "或": true, "一个": true, "这个": true, "那个": true,
	"什么": true, "怎么": true, "如何": true, "为什么": true,
}

// tokenize splits text into lowercase alphanumeric and CJK tokens.
// CJK characters are emitted as overlapping bigrams; stopwords are filtered.
func tokenize(text string) []string {
	var tokens []string
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	var cjkBuf []rune
	flushCJK := func() {
		if len(cjkBuf) == 0 {
			return
		}
		full := string(cjkBuf)
		// Filter single-stopword sequences.
		if !cjkStopwords[full] {
			tokens = append(tokens, full)
		}
		for i := 0; i+1 < len(cjkBuf); i++ {
			bigram := string([]rune{cjkBuf[i], cjkBuf[i+1]})
			if !cjkStopwords[bigram] {
				tokens = append(tokens, bigram)
			}
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

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}
