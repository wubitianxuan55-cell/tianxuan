package xai

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// OIDCDiscovery holds the OIDC endpoints discovered from xAI.
type OIDCDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

// OAuthConfig holds the XAI OAuth client configuration.
type OAuthConfig struct {
	ClientID   string
	ListenHost string
	ListenPort string
}

// DefaultOAuthConfig returns the default XAI OAuth config.
func DefaultOAuthConfig() OAuthConfig {
	return OAuthConfig{
		ClientID:   "b1a00492-073a-47ea-816f-4c329264a828",
		ListenHost: "127.0.0.1",
		ListenPort: "56121",
	}
}

// discoverEndpoints fetches the OIDC discovery document.
func discoverEndpoints() (*OIDCDiscovery, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get("https://auth.x.ai/.well-known/openid-configuration")
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil, fmt.Errorf("read OIDC discovery response: %w", err)
	}
	var disc OIDCDiscovery
	if err := json.Unmarshal(body, &disc); err != nil {
		return nil, fmt.Errorf("parse OIDC discovery: %w", err)
	}
	if disc.AuthorizationEndpoint == "" || disc.TokenEndpoint == "" {
		return nil, fmt.Errorf("OIDC discovery missing required fields")
	}
	return &disc, nil
}

// pkcePair holds a PKCE verifier and challenge.
type pkcePair struct {
	Verifier  string
	Challenge string
}

func newPKCE() (*pkcePair, error) {
	b, err := generateRandomBytes(32)
	if err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])
	return &pkcePair{Verifier: verifier, Challenge: challenge}, nil
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// ── OAuth Login Flow ──────────────────────────────────────────────

// LoginResult holds the OAuth result.
type LoginResult struct {
	Token   *Token
	BaseURL string
}

// DoLogin runs the full OAuth PKCE loopback flow.
func DoLogin(cfg OAuthConfig) (*LoginResult, error) {
	disc, err := discoverEndpoints()
	if err != nil {
		return nil, err
	}

	pkce, err := newPKCE()
	if err != nil {
		return nil, err
	}

	stateBytes, err := generateRandomBytes(16)
	if err != nil {
		return nil, err
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	nonceBytes, err := generateRandomBytes(16)
	if err != nil {
		return nil, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)

	redirectURI := fmt.Sprintf("http://%s:%s/callback", cfg.ListenHost, cfg.ListenPort)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			errCh <- fmt.Errorf("state mismatch — possible CSRF attack")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(oauthErrorHTML("State mismatch")))
			return
		}
		if ed := q.Get("error_description"); ed != "" {
			errCh <- fmt.Errorf("authorization failed: %s", ed)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(oauthErrorHTML(ed)))
			return
		}
		if ep := q.Get("error"); ep != "" {
			errCh <- fmt.Errorf("authorization failed: %s", ep)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(oauthErrorHTML(ep)))
			return
		}
		code := q.Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no authorization code received")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(oauthErrorHTML("No authorization code")))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(oauthSuccessHTML()))
		codeCh <- code
	})

	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.ListenHost, cfg.ListenPort))
	if err != nil {
		return nil, fmt.Errorf("start callback server (port %s): %w", cfg.ListenPort, err)
	}
	srv := &http.Server{Handler: mux}
	go func() { srv.Serve(listener) }()
	defer srv.Close()

	// Build auth URL
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid profile email api:access"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {pkce.Challenge},
		"code_challenge_method": {"S256"},
		"plan":                  {"generic"},
		"referrer":              {"tianxuan"},
	}
	authURL := disc.AuthorizationEndpoint + "?" + q.Encode()

	if err := openBrowser(authURL); err != nil {
		fmt.Printf("\nPlease open this URL to log in:\n\n%s\n\n", authURL)
	}

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("login timed out (5 minutes)")
	}

	token, err := exchangeCodeForToken(disc.TokenEndpoint, cfg.ClientID, redirectURI, code, pkce)
	if err != nil {
		return nil, err
	}

	return &LoginResult{Token: token, BaseURL: "https://api.x.ai/v1"}, nil
}

func exchangeCodeForToken(tokenEP, clientID, redirectURI, code string, pkce *pkcePair) (*Token, error) {
	payload := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {pkce.Verifier},
	}
	resp, err := http.PostForm(tokenEP, payload)
	if err != nil {
		return nil, fmt.Errorf("token exchange request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("token exchange denied (HTTP 403): account may not have API access — try setting XAI_API_KEY")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	token.ObtainedAt = time.Now()
	return &token, nil
}

// RefreshAccessToken uses the refresh token to get a new access token.
func RefreshAccessToken(clientID, refreshToken string) (*Token, error) {
	disc, err := discoverEndpoints()
	if err != nil {
		return nil, err
	}
	payload := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
	}
	resp, err := http.PostForm(disc.TokenEndpoint, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("refresh denied (HTTP 403): account may not have API access")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("refresh failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, err
	}
	token.ObtainedAt = time.Now()
	return &token, nil
}

// normalizeBaseURL validates the base URL to prevent credential leaks.
func normalizeBaseURL(rawURL string) string {
	candidate := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if candidate == "" {
		return "https://api.x.ai/v1"
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Scheme == "" {
		return "https://api.x.ai/v1"
	}
	if parsed.Scheme != "https" {
		return "https://api.x.ai/v1"
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "x.ai" && !strings.HasSuffix(host, ".x.ai") {
		return "https://api.x.ai/v1"
	}
	return candidate
}

// ── HTML pages ────────────────────────────────────────────────────

func oauthSuccessHTML() string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Login Successful — tianxuan</title>
<style>body{font-family:sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#0d0d0d;color:#e0e0e0}div{text-align:center}h1{color:#4ade80}p{color:#9ca3af}</style></head>
<body><div><h1>✓ Login Successful</h1><p>You may close this page and return to tianxuan.</p></div></body></html>`
}

func oauthErrorHTML(msg string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>Login Failed — tianxuan</title>
<style>body{font-family:sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#0d0d0d;color:#e0e0e0}div{text-align:center}h1{color:#f87171}p{color:#9ca3af}</style></head>
<body><div><h1>✗ Login Failed</h1><p>%s</p></div></body></html>`, msg)
}
