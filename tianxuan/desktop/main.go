// Command tianxuan-desktop is the Wails shell around the Tianxuan kernel: a native
// window hosting a webview frontend, with the Go-side control.Controller bound
// directly to the UI (no HTTP hop — bindings in, runtime events out). It lives in
// a nested module (tianxuan/desktop) so the CGO/WebKit desktop build never touches
// the CLI's CGO_ENABLED=0 single-static-binary guarantee, while still importing
// the same internal/* kernel.
package main

import (
	"embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"tianxuan/internal/crash"

	// Blank imports wire compile-time built-ins into their registries, exactly as
	// cmd/tianxuan does — boot.Build resolves providers/tools from these registries.
	_ "tianxuan/internal/provider/anthropic"
	_ "tianxuan/internal/provider/openai"
	_ "tianxuan/internal/provider/xai"
	_ "tianxuan/internal/tool/builtin"
)

// assets embeds the built frontend. `all:` so dotfiles (e.g. the dist .gitkeep
// that keeps this directive compilable before the first `pnpm build`) are
// included. A real run requires `pnpm build` (or `wails build`) to populate dist.
//
//go:embed all:frontend/dist
var assets embed.FS

// version is injected at build time via `wails build -ldflags "-X main.version=..."`,
// mirroring cmd/tianxuan/main.go. The auto-updater reads it (App.Version) to compare
// against the published manifest; an un-injected dev build stays "dev" and never
// prompts to update.
var version = "dev"

func main() {
	// Redirect stderr and slog to a log file — GUI apps (-H windowsgui) have no
	// console, so panics, crash.Recover, slog.Error etc. are silently lost without
	// this. The file lives under ~/.tianxuan/logs/ and is rotated per launch.
	logDir := filepath.Join(os.Getenv("USERPROFILE"), ".tianxuan", "logs")
	os.MkdirAll(logDir, 0700)
	logPath := filepath.Join(logDir, fmt.Sprintf("desktop-%s.log", time.Now().Format("20060102-150405")))
	logFile, logErr := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if logErr == nil {
		os.Stderr = logFile
		slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logFile, os.Stderr), nil)))
	}
	cleanOldLogs(logDir, 20)

	defer crash.Handle()
	app := NewApp()

	// Restore saved window size, or fall back to the default.
	width, height := 1400, 820
	if saved, ok := loadWindowState(); ok {
		if saved.Width > 0 {
			width = saved.Width
		}
		if saved.Height > 0 {
			height = saved.Height
		}
	}

	err := wails.Run(&options.App{
		Title:     "Tianxuan",
		Width:     width,
		Height:    height,
		MinWidth:  760,
		MinHeight: 480,
		// Match the dark UI shell so first paint (before CSS loads) doesn't flash
		// white — particularly visible on WebKitGTK.
		BackgroundColour:   &options.RGBA{R: 26, G: 26, B: 46, A: 255},
		AssetServer:        &assetserver.Options{Assets: assets},
		OnStartup:          app.startup,
		OnDomReady:         app.domReady,
		OnBeforeClose:      app.beforeClose,
		OnShutdown:         app.shutdown,
		Bind:               []any{app},
		SingleInstanceLock: singleInstanceLock(app),

		// Start hidden — domReady positions and shows the window after restoring
		// geometry, so the user never sees the default size/position flash.
		StartHidden: true,

		// Native OS file drops: the webview enables drag-and-drop for the
		// composer to receive file paths.
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true},

		// --- per-platform adaptation (see desktop/README.md for the rationale) ---
		Mac: &mac.Options{
			// Inset traffic-lights over a frameless-feeling header; the frontend
			// leaves a drag region at the top (CSS --wails-draggable).
			TitleBar: mac.TitleBarHiddenInset(),
			// Follow the OS appearance so the title bar matches light/dark system
			// preference instead of being locked to dark.
			Appearance: mac.DefaultAppearance,
		},
		Windows: &windows.Options{
			// Follow the OS light/dark setting; the frontend also honors
			// prefers-color-scheme so the two stay in sync.
			Theme: windows.SystemDefault,
		},
		Linux: &linux.Options{
			ProgramName: "Tianxuan",
			// WebKitGTK GPU compositing is inconsistent across distros/drivers and
			// is the one real cross-platform rough edge for a Go+webview stack:
			// "always" can yield blank or flickering webviews on some setups, so
			// we let the webview decide on demand. Users still hitting artifacts
			// can fall back to WEBKIT_DISABLE_COMPOSITING_MODE=1 (see README).
			WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}

// cleanOldLogs removes old log files, keeping only the most recent `keep` entries.
func cleanOldLogs(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) <= keep {
		return
	}
	for _, e := range entries[:len(entries)-keep] {
		if !e.IsDir() {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
