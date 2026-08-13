package builtin

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"tianxuan/internal/config"
)

// --- truncate ---

func TestTruncatePreservesUTF8(t *testing.T) {
	long := strings.Repeat("中文搜索能力测试", 100) // 800 runes
	out := truncate(long, 100)
	if !utf8.ValidString(out) {
		t.Fatalf("truncate produced invalid UTF-8: %q", out)
	}
	if got := utf8.RuneCountInString(out); got > 100 {
		t.Errorf("truncate returned %d runes, want <= 100", got)
	}
}

func TestTruncateShortString(t *testing.T) {
	s := "short"
	if out := truncate(s, 100); out != s {
		t.Errorf("truncate(%q) = %q, want unchanged", s, out)
	}
}

// --- language parameter ---

func TestSearchLanguageParam(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"golang generics tutorial", ""},
		{"今日天气", "zh-CN"},
		{"Go 语言教程", "zh-CN"},
		{"python とは", "zh-CN"},
	}
	for _, tt := range tests {
		if got := searchLanguageParam(tt.query); got != tt.want {
			t.Errorf("searchLanguageParam(%q) = %q, want %q", tt.query, got, tt.want)
		}
	}
}

// --- engine parsing (httptest-backed) ---

func TestBraveEngineDecodesGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Go's http.Transport adds Accept-Encoding: gzip itself and
		// decompresses transparently; the point is that the response parses.
		if got := r.Header.Get("Accept-Encoding"); got != "gzip" {
			t.Errorf("Accept-Encoding = %q, want auto gzip", got)
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_ = json.NewEncoder(gz).Encode(map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{"title": "Go", "url": "https://go.dev", "description": "The Go programming language"},
				},
			},
		})
		_ = gz.Close()
	}))
	defer srv.Close()

	eng := &braveEngine{apiKey: "test", baseURL: srv.URL}
	results, err := eng.Search(context.Background(), "golang", 5)
	if err != nil {
		t.Fatalf("brave engine failed on gzip response: %v", err)
	}
	if len(results) != 1 || results[0].URL != "https://go.dev" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestTavilyEngineParsesResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "T1", "url": "https://example.com/1", "content": "content one"},
				{"title": "T2", "url": "https://example.com/2", "content": "content two"},
			},
		})
	}))
	defer srv.Close()

	eng := &tavilyEngine{apiKey: "test", baseURL: srv.URL}
	results, err := eng.Search(context.Background(), "query", 1)
	if err != nil {
		t.Fatalf("tavily engine failed: %v", err)
	}
	if len(results) != 1 || results[0].Title != "T1" {
		t.Fatalf("topK not respected: %+v", results)
	}
}

func TestSearXNGEngineRetriesOnServerError(t *testing.T) {
	if testing.Short() {
		t.Skip("retry backoff; skipped under -short")
	}
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "S", "url": "https://example.com/s", "content": "searxng result"},
			},
		})
	}))
	defer srv.Close()

	eng := &localSearxNGEngine{baseURL: srv.URL}
	results, err := eng.Search(context.Background(), "hello", 5)
	if err != nil {
		t.Fatalf("searxng engine did not recover after retry: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSearXNGEngineReportsChallengePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>Verifying your browser…</title></head><body>challenge</body></html>`))
	}))
	defer srv.Close()

	eng := &localSearxNGEngine{baseURL: srv.URL}
	_, err := eng.Search(context.Background(), "hello", 5)
	if err == nil {
		t.Fatal("expected challenge-page error")
	}
	if !strings.Contains(err.Error(), "bot") && !strings.Contains(err.Error(), "challenge") {
		t.Fatalf("error = %v, want bot-protection hint", err)
	}
}

// --- Bing HTML fallback engine ---

func TestBingEngineParsesResults(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
<ol id="b_results">
  <li class="b_algo">
    <h2><a href="https://go.dev/doc/tutorial/generics">Go Generics Tutorial</a></h2>
    <div class="b_caption"><p>This tutorial introduces generics in Go.</p></div>
  </li>
  <li class="b_algo">
    <h2><a href="https://example.com/gen">Generics explained</a></h2>
    <div class="b_caption"><p>Explanation of generics with examples.</p></div>
  </li>
  <li class="b_ans">not a result</li>
</ol>
</body></html>`

	results, err := parseBingResults([]byte(html), 10)
	if err != nil {
		t.Fatalf("parseBingResults failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(results), results)
	}
	if results[0].Title != "Go Generics Tutorial" || results[0].URL != "https://go.dev/doc/tutorial/generics" {
		t.Errorf("first result wrong: %+v", results[0])
	}
	if !strings.Contains(results[0].Snippet, "generics") {
		t.Errorf("snippet missing: %+v", results[0])
	}
	if results[0].Source != "bing" {
		t.Errorf("source = %q, want bing", results[0].Source)
	}
}

func TestBingEngineParsesResultsRespectsLimit(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
<ol id="b_results">
  <li class="b_algo"><h2><a href="https://a.example">A</a></h2><p>a</p></li>
  <li class="b_algo"><h2><a href="https://b.example">B</a></h2><p>b</p></li>
  <li class="b_algo"><h2><a href="https://c.example">C</a></h2><p>c</p></li>
</ol>
</body></html>`

	results, err := parseBingResults([]byte(html), 2)
	if err != nil {
		t.Fatalf("parseBingResults failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestBingEngineNoResults(t *testing.T) {
	html := `<!DOCTYPE html><html><body><h1>No results</h1></body></html>`
	if _, err := parseBingResults([]byte(html), 10); err == nil {
		t.Fatal("expected error for empty result page")
	}
}

// --- SSRF guard / resolver handling ---

func TestResolveAllowedSearchIPs(t *testing.T) {
	orig := lookupSearchIPs
	defer func() { lookupSearchIPs = orig }()

	t.Run("rejects private IP", func(t *testing.T) {
		lookupSearchIPs = func(_ context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}, nil
		}
		ips, err := resolveAllowedSearchIPs(context.Background(), "example.com")
		if err == nil {
			t.Fatalf("expected SSRF rejection, got %v", ips)
		}
		if !strings.Contains(err.Error(), "refusing") {
			t.Errorf("error = %v, want refusal message", err)
		}
	})

	t.Run("rejects host mixing public and private IPs", func(t *testing.T) {
		lookupSearchIPs = func(_ context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{
				{IP: net.ParseIP("142.250.72.14")},
				{IP: net.ParseIP("10.0.0.1")},
			}, nil
		}
		if _, err := resolveAllowedSearchIPs(context.Background(), "example.com"); err == nil {
			t.Fatal("expected rejection for mixed private/public resolution (DNS rebinding)")
		}
	})

	t.Run("allows public-only resolution", func(t *testing.T) {
		lookupSearchIPs = func(_ context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("142.250.72.14")}}, nil
		}
		ips, err := resolveAllowedSearchIPs(context.Background(), "example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ips) != 1 || ips[0].String() != "142.250.72.14" {
			t.Fatalf("unexpected IPs: %v", ips)
		}
	})

	t.Run("empty resolution returns error, not panic", func(t *testing.T) {
		lookupSearchIPs = func(_ context.Context, host string) ([]net.IPAddr, error) {
			return nil, nil
		}
		if _, err := resolveAllowedSearchIPs(context.Background(), "example.com"); err == nil {
			t.Fatal("expected error for empty IP list")
		}
	})
}

// --- full tool execution ---

func TestWebSearchExecuteWithLocalEngine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query().Get("q"); !strings.Contains(q, "hello") {
			t.Errorf("query = %q, want contains hello", q)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Hello World", "url": "https://example.com/hello", "content": "hello world page"},
			},
		})
	}))
	defer srv.Close()

	origCfg := searchCfg
	defer func() { searchCfg = origCfg }()
	searchCfg = &config.SearchConfig{LocalSearXNGURL: srv.URL}

	ws := webSearch{}
	args, _ := json.Marshal(map[string]any{"query": "hello world", "topK": 3})
	out, err := ws.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(out, "https://example.com/hello") {
		t.Fatalf("output missing result URL:\n%s", out)
	}
}
