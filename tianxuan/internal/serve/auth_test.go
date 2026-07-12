package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	t1, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if len(t1) != 64 { // 32 bytes hex = 64 chars
		t.Fatalf("expected 64-char token, got %d", len(t1))
	}

	// Two calls should produce different tokens.
	t2, err := GenerateToken()
	if err != nil {
		t.Fatalf("second GenerateToken failed: %v", err)
	}
	if t1 == t2 {
		t.Fatal("expected different tokens from successive calls")
	}
}

func TestTokenAuthMiddleware_NoToken(t *testing.T) {
	// Empty token → no auth, should pass through.
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := tokenAuthMiddleware("", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestTokenAuthMiddleware_ExemptPaths(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := tokenAuthMiddleware("secret", okHandler)

	exempt := []string{"/", "/health", "/assets/main.js", "/assets/css/app.css", "/mobile", "/mobile/index.html"}
	for _, path := range exempt {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("exempt path %s: expected 200, got %d", path, rec.Code)
		}
	}
}

func TestTokenAuthMiddleware_MissingToken(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := tokenAuthMiddleware("secret", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestTokenAuthMiddleware_HeaderToken(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := tokenAuthMiddleware("my-secret-token", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestTokenAuthMiddleware_QueryToken(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := tokenAuthMiddleware("secret-query", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/history?token=secret-query", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with query token, got %d", rec.Code)
	}
}

func TestTokenAuthMiddleware_WrongToken(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := tokenAuthMiddleware("correct", okHandler)

	tests := []struct {
		name string
		fn   func() *http.Request
	}{
		{"wrong header", func() *http.Request {
			req := httptest.NewRequest(http.MethodGet, "/history", nil)
			req.Header.Set("Authorization", "Bearer wrong")
			return req
		}},
		{"wrong query", func() *http.Request {
			return httptest.NewRequest(http.MethodGet, "/history?token=wrong", nil)
		}},
		{"empty bearer", func() *http.Request {
			req := httptest.NewRequest(http.MethodGet, "/history", nil)
			req.Header.Set("Authorization", "Bearer ")
			return req
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, tt.fn())
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for %s, got %d", tt.name, rec.Code)
			}
		})
	}
}

func TestExtractToken(t *testing.T) {
	// Header extraction
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer abc123")
	if got := extractToken(req); got != "abc123" {
		t.Errorf("header: expected 'abc123', got '%s'", got)
	}

	// Query extraction (takes priority? No, header first)
	req2 := httptest.NewRequest(http.MethodGet, "/events?token=qwerty", nil)
	if got := extractToken(req2); got != "qwerty" {
		t.Errorf("query: expected 'qwerty', got '%s'", got)
	}

	// No token
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := extractToken(req3); got != "" {
		t.Errorf("none: expected '', got '%s'", got)
	}
}

func TestAuthExempt(t *testing.T) {
	tests := []struct {
		path   string
		exempt bool
	}{
		{"/", true},
		{"/health", true},
		{"/assets/main.js", true},
		{"/assets/sub/dir/file.css", true},
		{"/mobile", true},
		{"/mobile/index.html", true},
		{"/history", false},
		{"/submit", false},
		{"/events", false},
		{"/api/whatever", false},
	}

	for _, tt := range tests {
		if got := authExempt(tt.path); got != tt.exempt {
			t.Errorf("authExempt(%q) = %v, want %v", tt.path, got, tt.exempt)
		}
	}
}
