// Package serve exposes a control.Controller over HTTP: the typed event stream
// as Server-Sent Events, and the commands as small JSON POST endpoints. It is a
// second frontend alongside the chat TUI — proof that the controller is
// transport-agnostic, and the basis for a browser/desktop client. One server
// drives one session; multiple browser tabs share it.
package serve

import (
	"embed"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"tianxuan/internal/agent"
	"tianxuan/internal/agent/session"
	"tianxuan/internal/control"
	"tianxuan/internal/crash"
	"tianxuan/internal/provider"
)

//go:embed webui/index.html
var indexHTML []byte

//go:embed webui
var webDist embed.FS

// Server wires a controller to its HTTP surface. The Broadcaster must be the
// same sink the controller was constructed with, so events reach SSE clients.
type Server struct {
	ctrl      *control.Controller
	bc        *Broadcaster
	rebuildFn func() (*control.Controller, error)
	model     string
	maxSteps  int
	token     string // auth token (empty = no auth, localhost mode)
	public    bool   // bind 0.0.0.0 + wildcard CORS (requires token)
}

// New builds a Server. bc must be the controller's event sink.
func New(ctrl *control.Controller, bc *Broadcaster) *Server {
	return &Server{ctrl: ctrl, bc: bc}
}

// WithRebuild attaches a controller-rebuild function (e.g. boot.Build) so
// POST /rebuild can hot-reload settings. model/maxSteps are recorded for
// the settings view.
func (s *Server) WithRebuild(fn func() (*control.Controller, error), model string, maxSteps int) *Server {
	s.rebuildFn = fn
	s.model = model
	s.maxSteps = maxSteps
	return s
}

// WithToken enables token authentication. When set, all API endpoints require
// the token via Authorization: Bearer <token> or ?token=<token> query param.
func (s *Server) WithToken(token string) *Server {
	s.token = token
	return s
}

// WithPublic enables public (remote) mode: wildcard CORS headers so any
// origin can connect. Must be combined with WithToken for security.
func (s *Server) WithPublic(v bool) *Server {
	s.public = v
	return s
}

// Token returns the configured auth token (empty if auth is disabled).
func (s *Server) Token() string { return s.token }

// Handler returns the full HTTP handler chain: routes → auth (if configured)
// → csrf guard → logging. When public mode is on, wildcard CORS wraps
// everything.
func (s *Server) Handler() http.Handler {
	h := s.handler()
	if s.public {
		h = corsPublicMiddleware(h)
	}
	return h
}

// HandlerWithCORS returns the same routes as Handler but adds CORS headers
// for a specific origin (e.g. a dev Vite frontend on :5173). When public
// mode is on this is equivalent to Handler().
func (s *Server) HandlerWithCORS(origin string) http.Handler {
	if s.public {
		return s.Handler()
	}
	return corsMiddleware(s.handler(), origin)
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("GET /app.css", s.staticCSS)
	mux.HandleFunc("GET /app.js", s.staticJS)
	mux.HandleFunc("GET /events", s.events)
	mux.HandleFunc("GET /history", s.history)
	mux.HandleFunc("GET /context", s.context)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /submit", s.submit)
	mux.HandleFunc("POST /cancel", s.cancel)
	mux.HandleFunc("POST /approve", s.approve)
	mux.HandleFunc("POST /compact", s.compact)
	mux.HandleFunc("POST /new", s.newSession)
	mux.HandleFunc("GET /meta", s.meta)
	mux.HandleFunc("GET /memory", s.memory)
	mux.HandleFunc("POST /remember", s.remember)
	mux.HandleFunc("POST /forget", s.forget)
	mux.HandleFunc("POST /save-doc", s.saveDoc)
	mux.HandleFunc("POST /answer", s.answer)
	mux.HandleFunc("GET /models", s.models)
	mux.HandleFunc("GET /sessions", s.sessions)
	mux.HandleFunc("POST /delete-session", s.deleteSession)
	mux.HandleFunc("POST /resume-session", s.resumeSession)
	mux.HandleFunc("GET /files", s.listDir)
	mux.HandleFunc("GET /file", s.readFile)
	mux.HandleFunc("GET /balance", s.balance)
	mux.HandleFunc("GET /jobs", s.jobs)
	mux.HandleFunc("GET /commands", s.commands)
	mux.HandleFunc("GET /capabilities", s.capabilities)
	mux.HandleFunc("GET /tcca-report", s.tccaReport)
	mux.HandleFunc("POST /rebuild", s.rebuildHandler)
	mux.HandleFunc("GET /checkpoints", s.checkpoints)
	mux.HandleFunc("POST /checkpoints/rewind", s.rewindCheckpoint)
	mux.HandleFunc("POST /checkpoints/fork", s.forkCheckpoint)
	mux.HandleFunc("POST /checkpoints/summarize-from", s.summarizeFrom)
	mux.HandleFunc("POST /checkpoints/summarize-up-to", s.summarizeUpTo)
	mux.HandleFunc("POST /rename-session", s.renameSession)
	mux.HandleFunc("GET /slash-args", s.slashArgs)
	mux.HandleFunc("GET /settings", s.settings)
	mux.HandleFunc("POST /settings/bypass", s.setBypass)
	mux.HandleFunc("POST /settings/model", s.setModel)
	mux.HandleFunc("POST /settings/default-model", s.setDefaultModel)
	mux.HandleFunc("POST /settings/provider", s.saveProvider)
	mux.HandleFunc("POST /settings/delete-provider", s.deleteProvider)
	mux.HandleFunc("POST /settings/provider-key", s.setProviderKey)
	mux.HandleFunc("POST /settings/agent-params", s.setAgentParams)
	mux.HandleFunc("POST /settings/sandbox", s.setSandbox)
	mux.HandleFunc("POST /settings/permission-mode", s.setPermissionMode)
	mux.HandleFunc("POST /settings/permission-rule", s.addPermissionRule)
	mux.HandleFunc("DELETE /settings/permission-rule", s.removePermissionRule)
	mux.HandleFunc("POST /mcp/add", s.addMCPServer)
	mux.HandleFunc("POST /mcp/remove", s.removeMCPServer)
	mux.HandleFunc("POST /mcp/retry", s.retryMCPServer)
	mux.HandleFunc("POST /mcp/enabled", s.setMCPServerEnabled)
	mux.Handle("GET /assets/", http.FileServer(http.FS(webDist)))

	// Mobile SPA routes — embedded at build time via go:embed mobileui
	if mobileDist, ok := tryMobileDist(); ok {
		mux.HandleFunc("GET /mobile", s.indexMobile)
		mux.Handle("GET /mobile/", http.StripPrefix("/mobile", http.FileServer(http.FS(mobileDist))))
	}

	h := http.Handler(mux)
	// Middleware stack (innermost → outermost):
	//   1. csrfGuard — require application/json on POST
	//   2. tokenAuth — validate token (no-op when s.token is empty)
	//   3. logMiddleware — request logging
	h = csrfGuard(h)
	h = tokenAuthMiddleware(s.token, h)
	h = logMiddleware(h)
	return h
}

// csrfGuard rejects state-changing requests that don't carry a JSON content type.
// The command endpoints have no auth and bind to localhost, so a page the user
// visits could otherwise drive them with a simple cross-origin POST (text/plain,
// no preflight) — submitting prompts or auto-approving tool calls. Requiring
// application/json forces a CORS preflight the unauthenticated server never
// answers, blocking cross-site requests; the same-origin frontend (which always
// sends JSON) is unaffected.
func csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			ct := r.Header.Get("Content-Type")
			if i := strings.IndexByte(ct, ';'); i >= 0 {
				ct = ct[:i]
			}
			if strings.TrimSpace(ct) != "application/json" {
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Run serves until the process is killed. Interactive approval is enabled so
// "ask" decisions surface as approval_request events answered via POST /approve.
func (s *Server) Run(addr string) error {
	s.ctrl.EnableInteractiveApproval()
	return http.ListenAndServe(addr, s.Handler())
}

// RunGraceful serves with graceful shutdown. It listens for SIGINT/SIGTERM on
// the provided context and drains active connections for up to 10 seconds
// before returning.
func (s *Server) RunGraceful(ctx context.Context, addr string) error {
	s.ctrl.EnableInteractiveApproval()
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		defer crash.Recover("serve-listen")
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("serve: shutting down gracefully")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("serve: graceful shutdown failed", "err", err)
		}
		return <-errCh
	}
}

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

// indexMobile serves the mobile SPA entrypoint at /mobile. Falls back to
// 404 when the mobile frontend has not been built (z_embed_mobile.go absent).
func (s *Server) indexMobile(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if mobileHTML, ok := getMobileIndex(); ok {
		_, _ = w.Write(mobileHTML)
		return
	}
	http.NotFound(w, nil)
}

// staticCSS serves the client stylesheet.
func (s *Server) staticCSS(w http.ResponseWriter, _ *http.Request) {
	http.NotFound(w, nil)
}

// staticJS serves the client script.
func (s *Server) staticJS(w http.ResponseWriter, _ *http.Request) {
	http.NotFound(w, nil)
}

// health returns 200 with a JSON heartbeat so the frontend can detect liveness
// independently of the SSE stream (which may reconnect).
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "time": time.Now().UnixMilli()})
}

// Rebuild re-creates the controller from current config, carrying the
// conversation forward (same pattern as desktop/settings_app.go rebuild).
func (s *Server) Rebuild() error {
	if s.rebuildFn == nil {
		return fmt.Errorf("rebuild not available")
	}
	var carried []provider.Message
	var sessionDir string
	if s.ctrl != nil {
		_ = s.ctrl.Snapshot()
		carried = s.ctrl.History()
		sessionDir = s.ctrl.SessionDir()
		s.ctrl.Close()
	}
	ctrl, err := s.rebuildFn()
	if err != nil {
		return err
	}
	s.ctrl = ctrl
	s.ctrl.EnableInteractiveApproval()
	if len(carried) > 0 {
		s.ctrl.Resume(&agent.Session{Messages: carried}, "")
	}
	if sessionDir != "" {
		s.ctrl.SetSessionPath(agent.NewSessionPath(sessionDir, s.ctrl.Label()))
	}
	return nil
}

// rebuildHandler runs Rebuild on demand and returns the new meta.
func (s *Server) rebuildHandler(w http.ResponseWriter, _ *http.Request) {
	if err := s.Rebuild(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.meta(w, nil)
}

// events streams the controller's event flow as SSE until the client
// disconnects. Each event is one `data:` frame of the JSON wire form.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsubscribe := s.bc.Subscribe()
	defer unsubscribe()

	fmt.Fprint(w, ": connected\n\n") // open the stream immediately
	flusher.Flush()

	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// submit runs raw user input as a turn (slash commands and @-references
// resolved by the controller). Returns 202 — output arrives on the event stream.
// Also broadcasts the user message to all SSE clients so every connected
// viewer (desktop web, mobile) sees the input immediately.
func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Input == "" {
		http.Error(w, "missing input", http.StatusBadRequest)
		return
	}
	s.ctrl.Submit(body.Input)

	// Broadcast user message to all SSE clients for real-time sync
	userMsg, _ := json.Marshal(map[string]string{
		"kind": "user_message",
		"text": body.Input,
	})
	s.bc.BroadcastRaw(userMsg)

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) cancel(w http.ResponseWriter, _ *http.Request) {
	s.ctrl.Cancel()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string `json:"id"`
		Allow   bool   `json:"allow"`
		Session bool   `json:"session"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	s.ctrl.Approve(body.ID, body.Allow, body.Session)
	w.WriteHeader(http.StatusNoContent)
}


func (s *Server) compact(w http.ResponseWriter, r *http.Request) {
	if err := s.ctrl.Compact(r.Context(), ""); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) newSession(w http.ResponseWriter, _ *http.Request) {
	if err := s.ctrl.NewSession(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// history returns the session's message log as {role, content} pairs so a
// reconnecting client can repopulate its transcript.
func (s *Server) history(w http.ResponseWriter, _ *http.Request) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var out []msg
	for _, m := range s.ctrl.History() {
		content := m.Content
		if m.Role == provider.RoleUser {
			content = session.StripTransientBlocks(m.Content)
			if strings.HasPrefix(strings.TrimSpace(content), "<compaction-summary>") {
				content = "〈会话摘要〉"
			}
		}
		out = append(out, msg{Role: string(m.Role), Content: content})
	}
	writeJSON(w, out)
}

// context returns the prompt-vs-window gauge numbers plus a percent so the
// frontend can draw a context-window meter without computing it client-side.
func (s *Server) context(w http.ResponseWriter, _ *http.Request) {
	used, window := s.ctrl.ContextSnapshot()
	pct := 0
	if window > 0 {
		pct = used * 100 / window
	}
	writeJSON(w, map[string]any{
		"used":    used,
		"window":  window,
		"percent": pct,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("serve: writeJSON encode failed", "err", err)
	}
}

// corsMiddleware adds CORS headers for a specific allowed origin. Only use for
// local development — the server has no auth, so broad CORS would let any site
// drive the agent. origin is the exact origin to allow (e.g.
// "http://localhost:5173"); empty origin skips CORS entirely.
func corsMiddleware(next http.Handler, origin string) http.Handler {
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// corsPublicMiddleware adds wildcard CORS headers for public (remote) mode.
// Only used when --public is set AND token auth is active, so the security
// risk is mitigated by the token requirement.
func corsPublicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// logMiddleware logs each request's method, path, and status.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("serve: request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", time.Since(start).String(),
		)
	})
}

// responseWriter captures the status code for logging.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush delegates to the underlying ResponseWriter if it supports flushing
// (required for SSE /events). Without this the type assertion in the events
// handler fails and the stream endpoint returns 500.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
