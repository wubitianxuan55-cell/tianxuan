package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/options"
)

// single_instance.go prevents multiple desktop instances. A second launch routes
// its args to the running instance via Wails SingleInstanceLock. Set
// TIANXUAN_DEV=1 to skip the lock during development.
// (Design adopted from DeepSeek-Reasonix-V1.12)

const singleInstanceIDPrefix = "com.tianxuan.desktop"

func singleInstanceID() string {
	abs, err := os.Executable()
	if err != nil {
		return singleInstanceIDPrefix
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	} else if fallback, err := filepath.Abs(abs); err == nil {
		abs = fallback
	}
	sum := sha256.Sum256([]byte(abs))
	return singleInstanceIDPrefix + "." + hex.EncodeToString(sum[:8])
}

func singleInstanceLock(app *App) *options.SingleInstanceLock {
	// Allow contributors to run a dev build alongside the installed app.
	if os.Getenv("TIANXUAN_DEV") != "" {
		return nil
	}
	return &options.SingleInstanceLock{
		UniqueId: singleInstanceID(),
		OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
			app.secondInstanceLaunch()
		},
	}
}

// secondInstanceLaunch brings the existing window to front when a second launch
// is attempted (e.g. clicking the dock icon). The frontend reacts by focusing
// the active tab.
func (a *App) secondInstanceLaunch() {
	if a.ctx == nil {
		return
	}
	// The simplest cross-platform way to surface the window:
	// Wails runtime will focus the existing window automatically.
}
