package xai

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Token represents an XAI OAuth token pair. Persisted to TokenStorePath().
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	Scope        string    `json:"scope"`
	ObtainedAt   time.Time `json:"obtained_at"`
}

// IsExpired checks if the access token is expired (with 1-hour buffer — xAI tokens last ~6h).
func (t *Token) IsExpired() bool {
	if t == nil || t.AccessToken == "" {
		return true
	}
	if t.ExpiresIn <= 0 {
		return false
	}
	exp := t.ObtainedAt.Add(time.Duration(t.ExpiresIn) * time.Second)
	return time.Now().Add(1 * time.Hour).After(exp)
}

// TokenStore provides thread-safe file persistence for OAuth tokens (0600 permissions).
type TokenStore struct {
	mu   sync.RWMutex
	path string
}

func NewTokenStore(path string) *TokenStore {
	return &TokenStore{path: path}
}

func (s *TokenStore) Save(t *Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize token: %w", err)
	}
	return os.WriteFile(s.path, data, 0600)
}

func (s *TokenStore) Load() (*Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var t Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *TokenStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
