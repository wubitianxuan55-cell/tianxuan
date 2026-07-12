// Package serve — Token authentication middleware for remote mobile access.
//
// When a token is configured (non-empty), all API endpoints require
// authentication via Authorization: Bearer <token> header or ?token=<token>
// query parameter.
//
// Exempt paths (no auth required):
//   GET /            — landing page
//   GET /health      — health check
//   GET /assets/*    — static assets (CSS, JS, fonts)
//   GET /mobile      — mobile SPA
//   GET /mobile/*    — mobile SPA assets
//
// The query-parameter channel exists because EventSource (SSE) does not
// support custom headers — mobile clients pass the token as ?token=... on
// the /events endpoint.
package serve

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

// GenerateToken produces a cryptographically random 32-byte hex token (64
// characters). Suitable for one-off session tokens printed at serve startup.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// authExempt reports whether path does not require token authentication.
func authExempt(path string) bool {
	if path == "/" || path == "/health" {
		return true
	}
	if strings.HasPrefix(path, "/assets/") {
		return true
	}
	if path == "/mobile" || strings.HasPrefix(path, "/mobile/") {
		return true
	}
	return false
}

// extractToken returns the bearer token from an HTTP request, checking (in
// order) the Authorization: Bearer header, then the ?token= query parameter.
func extractToken(r *http.Request) string {
	// 1. Authorization: Bearer <token>
	if auth := r.Header.Get("Authorization"); auth != "" {
		if t, ok := strings.CutPrefix(auth, "Bearer "); ok {
			return strings.TrimSpace(t)
		}
	}
	// 2. ?token=<token> query parameter
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return ""
}

// tokenAuthMiddleware wraps handler with token validation. When token is
// empty, requests pass through unauthenticated (backwards-compatible
// localhost mode).
func tokenAuthMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next // no auth — localhost mode
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authExempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if extractToken(r) != token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid or missing token"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
