package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tianxuan/internal/serve"
)

// MobileAccessView is the status blob returned to Settings > Mobile.
type MobileAccessView struct {
	Running      bool   `json:"running"`
	URL          string `json:"url"`      // LAN URL
	PublicURL    string `json:"publicUrl"` // ngrok public URL (empty if not using ngrok)
	Token        string `json:"token"`
	Port         int    `json:"port"`
	UsingNgrok   bool   `json:"usingNgrok"`
	NgrokReady   bool   `json:"ngrokReady"` // true once tunnel is established
}

type mobileServer struct {
	srv       *http.Server
	token     string
	port      int
	cancel    context.CancelFunc
	ngrokCmd  *exec.Cmd       // non-nil when ngrok is active
	publicURL string          // populated once ngrok tunnel is up
}

var mobileMu sync.Mutex
var mobileInst *mobileServer

// CheckNgrok returns whether the ngrok binary is found in PATH.
func (a *App) CheckNgrok() bool {
	_, err := exec.LookPath("ngrok")
	return err == nil
}

// MobileAccessStatus returns the current mobile access server state.
func (a *App) MobileAccessStatus() MobileAccessView {
	mobileMu.Lock()
	defer mobileMu.Unlock()
	if mobileInst == nil {
		return MobileAccessView{}
	}
	ua := mobileInst.publicURL != ""
	return MobileAccessView{
		Running:    true,
		URL:        fmt.Sprintf("http://%s:%d/mobile?token=%s", localIP(), mobileInst.port, mobileInst.token),
		PublicURL:  mobileInst.publicURL,
		Token:      mobileInst.token,
		Port:       mobileInst.port,
		UsingNgrok: ua,
		NgrokReady: mobileInst.publicURL != "",
	}
}

// StartMobileAccess starts an HTTP+SSE server. If ngrokToken is non-empty,
// ngrok is launched to provide a public internet URL.
func (a *App) StartMobileAccess(port int, ngrokToken string) (MobileAccessView, error) {
	mobileMu.Lock()
	defer mobileMu.Unlock()

	if mobileInst != nil {
		return MobileAccessView{}, fmt.Errorf("移动端已启动 (端口 %d)", mobileInst.port)
	}

	ctrl := a.activeCtrlLocked()
	if ctrl == nil {
		return MobileAccessView{}, fmt.Errorf("没有活跃会话")
	}

	token, err := serve.GenerateToken()
	if err != nil {
		return MobileAccessView{}, fmt.Errorf("令牌生成失败: %w", err)
	}

	if port <= 0 {
		port = 8787
	}
	listenAddr := fmt.Sprintf("0.0.0.0:%d", port)

	bc := serve.NewBroadcaster()
	ctrl.EnableInteractiveApproval()

	srvInst := serve.New(ctrl, bc).
		WithToken(token).
		WithPublic(true)

	httpSrv := &http.Server{
		Addr:              listenAddr,
		Handler:           srvInst.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start HTTP server
	a.goSafeQuiet("mobile-http-server", func() {
		slog.Info("desktop: 移动端 HTTP 服务已启动", "addr", listenAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Warn("desktop: 移动端 HTTP 服务出错", "err", err)
		}
		mobileMu.Lock()
		if mobileInst != nil && mobileInst.ngrokCmd != nil {
			_ = mobileInst.ngrokCmd.Process.Kill()
		}
		mobileInst = nil
		mobileMu.Unlock()
	})

	a.goSafeQuiet("mobile-shutdown-waiter", func() {
		<-ctx.Done()
		shutdownCtx, sc := context.WithTimeout(context.Background(), 5*time.Second)
		defer sc()
		_ = httpSrv.Shutdown(shutdownCtx)
	})

	ms := &mobileServer{
		srv:    httpSrv,
		token:  token,
		port:   port,
		cancel: cancel,
	}

	// Wire broadcaster
	a.mu.Lock()
	if a.sink != nil {
		a.sink.bc = bc
	}
	a.mu.Unlock()

	result := MobileAccessView{
		Running: true,
		URL:     fmt.Sprintf("http://%s:%d/mobile?token=%s", localIP(), port, token),
		Token:   token,
		Port:    port,
	}

	// Start ngrok if token provided
	if ngrokToken != "" {
		if _, err := exec.LookPath("ngrok"); err != nil {
			slog.Warn("desktop: ngrok 未安装，跳过外网穿透")
			mobileInst = ms
			return result, fmt.Errorf("ngrok 未安装，请先从 https://ngrok.com 下载并加入 PATH")
		}

		// Configure ngrok auth
		authCmd := exec.Command("ngrok", "config", "add-authtoken", ngrokToken)
		if out, err := authCmd.CombinedOutput(); err != nil {
			slog.Warn("desktop: ngrok 配置失败", "err", err, "out", string(out))
			mobileInst = ms
			return result, fmt.Errorf("ngrok token 配置失败: %s", string(out))
		}

		// Start ngrok tunnel
		ngrokCtx, ngrokCancel := context.WithCancel(ctx)
		ms.ngrokCmd = exec.CommandContext(ngrokCtx, "ngrok", "http", fmt.Sprintf("%d", port), "--log=stderr")
		ms.ngrokCmd.Stderr = nil
		if err := ms.ngrokCmd.Start(); err != nil {
			slog.Warn("desktop: ngrok 启动失败", "err", err)
			ngrokCancel()
			mobileInst = ms
			return result, fmt.Errorf("ngrok 启动失败: %w", err)
		}

		// Kill ngrok on cancel
		a.goSafeQuiet("mobile-ngrok-cancel", func() {
			<-ctx.Done()
			ngrokCancel()
		})

		// Poll ngrok local API for public URL
		result.UsingNgrok = true
		a.goSafeQuiet("mobile-ngrok-poll", func() {
			pollNgrok(ctx, ms, port)
		})
	}

	mobileInst = ms

	// Persist token and port for auto-start
	saveMobileConfig(token, port)

	return result, nil
}

// pollNgrok polls the ngrok local API until a tunnel URL is available.
func pollNgrok(ctx context.Context, ms *mobileServer, port int) {
	apiURL := "http://127.0.0.1:4040/api/tunnels"
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(15 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			slog.Warn("desktop: ngrok 隧道超时")
			return
		case <-ticker.C:
			resp, err := client.Get(apiURL)
			if err != nil {
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var data struct {
				Tunnels []struct {
					PublicURL string `json:"public_url"`
				} `json:"tunnels"`
			}
			if err := json.Unmarshal(body, &data); err != nil {
				continue
			}
			for _, t := range data.Tunnels {
				if t.PublicURL != "" && t.PublicURL != "null" {
					ms.publicURL = t.PublicURL + "/mobile?token=" + ms.token
					slog.Info("desktop: ngrok 隧道就绪", "url", ms.publicURL)
					return
				}
			}
		}
	}
}

// StopMobileAccess shuts down the mobile access server and ngrok tunnel.
func (a *App) StopMobileAccess() error {
	mobileMu.Lock()
	defer mobileMu.Unlock()

	if mobileInst == nil {
		return nil
	}

	a.mu.Lock()
	if a.sink != nil {
		a.sink.bc = nil
	}
	a.mu.Unlock()

	// Kill ngrok first
	if mobileInst.ngrokCmd != nil && mobileInst.ngrokCmd.Process != nil {
		_ = mobileInst.ngrokCmd.Process.Kill()
	}

	mobileInst.cancel()
	mobileInst = nil
	slog.Info("desktop: 移动端服务已停止")
	return nil
}

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}

// ── Token 持久化 ─────────────────────────────────────────────────────

func mobileConfigDir() string {
	dir, _ := os.UserHomeDir()
	return filepath.Join(dir, ".tianxuan")
}

func saveMobileConfig(token string, port int) {
	dir := mobileConfigDir()
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "mobile-token"), []byte(token), 0600)
	os.WriteFile(filepath.Join(dir, "mobile-port"), []byte(fmt.Sprintf("%d", port)), 0600)
}

func loadMobileConfig() (token string, port int) {
	dir := mobileConfigDir()
	b, _ := os.ReadFile(filepath.Join(dir, "mobile-token"))
	token = strings.TrimSpace(string(b))
	b, _ = os.ReadFile(filepath.Join(dir, "mobile-port"))
	if b != nil {
		fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &port)
	}
	if port == 0 {
		port = 8787
	}
	return
}

// AutoStartMobileAccess restores the last config and starts the server.
// Returns nil if no saved config exists.
func (a *App) AutoStartMobileAccess() *MobileAccessView {
	token, port := loadMobileConfig()
	if token == "" {
		return nil
	}
	slog.Info("desktop: 自动恢复移动端服务", "port", port)
	result, err := a.StartMobileAccess(port, "")
	if err != nil {
		slog.Warn("desktop: 自动恢复失败", "err", err)
		return nil
	}
	return &result
}

// GetPersistedMobileToken returns the saved token (if any).
func (a *App) GetPersistedMobileToken() string {
	token, _ := loadMobileConfig()
	return token
}
