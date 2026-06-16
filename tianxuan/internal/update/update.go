// Package update provides self-update for tianxuan. It fetches the latest
// release from a GitHub repository and replaces the running binary.
//
// Usage:
//
//	err := update.Self(context.Background(), "tianxuanX/tianxuan", "v7.3.0")
//	// or specify a version to check against
package update

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// GitHubRelease represents a GitHub release API response (minimal subset).
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
	PublishedAt string `json:"published_at"`
}

// Self performs a self-update: queries the GitHub releases for repo, finds the
// asset matching the current platform, downloads and replaces the running binary.
// currentVersion is the version from `version` ldflag, used to skip re-download
// when already on the latest.
//
// When releaseTag is not empty, Self downloads that specific release instead of
// the latest.
func Self(ctx context.Context, repo, currentVersion, releaseTag string) error {
	if currentVersion == "" || currentVersion == "dev" {
		return fmt.Errorf("cannot auto-update a dev build (no version tag); build with -ldflags '-X main.version=vX.Y.Z', or download manually from https://github.com/%s/releases", repo)
	}

	// 1. Fetch release info
	latest, err := FetchLatestRelease(ctx, repo, releaseTag)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	// 2. Compare versions
	latestTag := strings.TrimPrefix(latest.TagName, "v")
	currTag := strings.TrimPrefix(currentVersion, "v")
	if releaseTag == "" && compareVersions(currTag, latestTag) >= 0 {
		fmt.Fprintf(os.Stderr, "✓ tianxuan %s is up to date (latest: %s)\n", currentVersion, latest.TagName)
		return nil
	}

	// 3. Find matching asset
	assetName := assetName()
	if assetName == "" {
		return fmt.Errorf("update: unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	var downloadURL string
	for _, a := range latest.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		// Try matching by suffix
		for _, a := range latest.Assets {
			if strings.HasSuffix(a.Name, assetName) {
				downloadURL = a.BrowserDownloadURL
				break
			}
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("update: no asset matching %q found in release %s", assetName, latest.TagName)
	}

	// 4. Download
	fmt.Fprintf(os.Stderr, "⬇ downloading tianxuan %s (%s)...\n", latest.TagName, assetName)
	body, err := downloadWithProgress(ctx, downloadURL)
	if err != nil {
		return fmt.Errorf("update: download: %w", err)
	}

	// 5. Get current binary path
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("update: cannot determine executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("update: cannot resolve symlinks: %w", err)
	}

	// 6. Write new binary to a temp file, then rename (atomic on Unix, best-effort on Windows)
	tmp := exe + ".update.tmp"
	if err := os.WriteFile(tmp, body, 0o755); err != nil {
		return fmt.Errorf("update: write temp: %w", err)
	}

	// On Windows, we can't overwrite the running exe, so we need a rename trick
	if runtime.GOOS == "windows" {
		old := exe + ".old"
		_ = os.Remove(old) // ignore error if .old doesn't exist
		if err := os.Rename(exe, old); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("update: rename current → .old: %w (try closing tianxuan first)", err)
		}
		if err := os.Rename(tmp, exe); err != nil {
			// Attempt rollback
			_ = os.Rename(old, exe)
			_ = os.Remove(tmp)
			return fmt.Errorf("update: rename new → current: %w; original binary restored", err)
		}
		fmt.Fprintf(os.Stderr, "✓ updated to tianxuan %s (old binary saved as %s)\n", latest.TagName, old)
	} else {
		if err := os.Rename(tmp, exe); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("update: rename: %w", err)
		}
		fmt.Fprintf(os.Stderr, "✓ updated to tianxuan %s\n", latest.TagName)
	}
	return nil
}

// FetchLatestRelease fetches the latest (or tagged) release info from GitHub API.
// It is exported so CLI commands and other callers can check for updates without
// triggering a download.
func FetchLatestRelease(ctx context.Context, repo, tag string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	if tag != "" {
		url = fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, tag)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	// GitHub API has a 60 req/h unauthenticated rate limit. Passing a token via
	// GITHUB_TOKEN env bumps it to 5000 req/h.
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, body)
	}
	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	return &release, nil
}

// downloadWithProgress downloads a URL and returns the raw bytes.
func downloadWithProgress(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var reader io.Reader = resp.Body
	if strings.HasSuffix(url, ".gz") || strings.HasSuffix(url, ".tgz") {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		defer gr.Close()
		reader = gr
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}

// assetName returns the expected release asset filename for the current platform.
// Pattern: tianxuan-<os>-<arch>[.exe]
func assetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}
	return fmt.Sprintf("tianxuan-%s-%s%s", runtime.GOOS, arch, ext)
}

// compareVersions compares two semver strings (vX.Y.Z). Returns -1/0/1.
func compareVersions(a, b string) int {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	maxLen := len(ap)
	if len(bp) > maxLen {
		maxLen = len(bp)
	}
	for i := 0; i < maxLen; i++ {
		var an, bn int
		if i < len(ap) {
			fmt.Sscanf(ap[i], "%d", &an)
		}
		if i < len(bp) {
			fmt.Sscanf(bp[i], "%d", &bn)
		}
		if an < bn {
			return -1
		}
		if an > bn {
			return 1
		}
	}
	return 0
}
