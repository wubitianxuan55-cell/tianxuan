// Package xai implements the XAI / Grok provider with OAuth PKCE login.
// Two authentication methods: OAuth PKCE (default, cached in ~/.tianxuan/) or XAI_API_KEY.
// Pattern adapted from gaeaW's internal/provider/xai/.
package xai

import (
	"crypto/rand"
	"os"
	"path/filepath"

	"tianxuan/internal/provider"
)

func init() {
	provider.Register("xai", New)
}

// TokenStorePath returns the XAI token path (~/.tianxuan/xai_token.json).
func TokenStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "xai_token.json"
	}
	td := filepath.Join(home, ".tianxuan")
	os.MkdirAll(td, 0700)
	return filepath.Join(td, "xai_token.json")
}

// tokenManager manages the lifecycle of XAI authentication tokens.
// Priority: API Key > cached OAuth token > login required.
type tokenManager struct {
	store  *TokenStore
	cfg    OAuthConfig
	token  *Token
	apiKey string
}

func newTokenManager(apiKey string) *tokenManager {
	tm := &tokenManager{
		store:  NewTokenStore(TokenStorePath()),
		cfg:    DefaultOAuthConfig(),
		apiKey: apiKey,
	}
	if stored, err := tm.store.Load(); err == nil && stored != nil {
		tm.token = stored
	}
	return tm
}

func (tm *tokenManager) getAccessToken() (string, error) {
	if tm.apiKey != "" {
		return tm.apiKey, nil
	}
	if tm.token != nil && !tm.token.IsExpired() {
		return tm.token.AccessToken, nil
	}
	if tm.token != nil && tm.token.RefreshToken != "" {
		nt, err := RefreshAccessToken(tm.cfg.ClientID, tm.token.RefreshToken)
		if err == nil {
			tm.token = nt
			tm.store.Save(nt)
			return nt.AccessToken, nil
		}
	}
	return "", errNotLoggedIn
}

func (tm *tokenManager) IsLoggedIn() bool {
	if tm.apiKey != "" {
		return true
	}
	return tm.token != nil && !tm.token.IsExpired()
}

func (tm *tokenManager) Login() error {
	result, err := DoLogin(tm.cfg)
	if err != nil {
		return err
	}
	tm.token = result.Token
	return tm.store.Save(result.Token)
}

func (tm *tokenManager) Logout() error {
	tm.token = nil
	return tm.store.Delete()
}

// ── Public API ─────────────────────────────────────────────────────

var errNotLoggedIn = &provider.AuthError{Provider: "xai", KeyEnv: "XAI_API_KEY (or OAuth login)", Status: 401}

// EnsureLogin checks XAI is usable.
func EnsureLogin() error {
	tm := newTokenManager("")
	if tm.IsLoggedIn() {
		return nil
	}
	return errNotLoggedIn
}

// Login triggers the XAI OAuth login flow.
func Login() error {
	tm := newTokenManager("")
	return tm.Login()
}

// Logout signs out of XAI.
func Logout() error {
	tm := newTokenManager("")
	return tm.Logout()
}

// IsLoggedIn reports whether XAI is authenticated.
func IsLoggedIn() bool {
	return newTokenManager("").IsLoggedIn()
}

func generateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
