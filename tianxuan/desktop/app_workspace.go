package main

import (
	"bytes"
	"fmt"
	goruntime "runtime"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tianxuan/internal/agent"
	"tianxuan/internal/boot"
	"tianxuan/internal/config"
	"tianxuan/internal/control"
)

// WorkspaceMeta summarises one workspace folder for the picker.
type WorkspaceMeta struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

// DirEntry is one entry in the "@" file-reference menu.
type DirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
}

// FilePreview is a bounded, read-only file payload for the workspace side panel.
type FilePreview struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
	Err       string `json:"err,omitempty"`
}

// WorkspaceChangeView is a single file change recorded during a session.
type WorkspaceChangeView struct {
	Path   string `json:"path"`
	Added  int    `json:"added"`
	Removed int   `json:"removed"`
}

// atSkip are entries the "@" menu hides as noise.
var atSkip = map[string]bool{".git": true, "node_modules": true, ".DS_Store": true}

const filePreviewLimit = 256 * 1024

func trimUTF8PartialSuffix(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	for i := len(data) - 1; i >= 0 && len(data)-i <= utf8.UTFMax; i-- {
		if !utf8.RuneStart(data[i]) {
			continue
		}
		if !utf8.Valid(data[:i]) || utf8.FullRune(data[i:]) {
			return data
		}
		return data[:i]
	}
	return data
}

func workspacePath(rel string) (string, bool, error) {
	base, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	if rel == "" {
		return "", false, os.ErrInvalid
	}
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, rel)
	}
	path = filepath.Clean(path)
	r, err := filepath.Rel(base, path)
	if err != nil {
		return "", false, err
	}
	if r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return "", false, os.ErrPermission
	}
	return path, true, nil
}

// PickWorkspace opens a folder chooser and, on a pick, switches the agent to that
// project: it re-roots the process there, rebuilds the controller from that
// folder's tianxuan.toml + TIANXUAN.md, and starts a fresh session — the desktop
// analogue of opening a different project. The new controller is built before the
// old one is torn down, so a folder whose config can't load leaves the current
// session untouched. Returns the chosen path ("" if cancelled).
func (a *App) PickWorkspace() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	cur, _ := os.Getwd()
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Choose working folder",
		DefaultDirectory: cur,
	})
	if err != nil || dir == "" {
		return "", err // cancelled or error → no change
	}
	return a.SwitchWorkspace(dir)
}

func (a *App) ListWorkspaces() []WorkspaceMeta {
	cur, _ := os.Getwd()
	seen := map[string]bool{}
	paths := make([]string, 0, 8)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		if seen[path] {
			return
		}
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}
	add(cur)
	for _, path := range loadWorkspaces() {
		add(path)
	}
	out := make([]WorkspaceMeta, 0, len(paths))
	for _, path := range paths {
		out = append(out, WorkspaceMeta{
			Path:    path,
			Name:    workspaceName(path),
			Current: path == cur,
		})
	}
	return out
}

func workspaceName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return path
	}
	return name
}

func (a *App) SwitchWorkspace(dir string) (string, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = home
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	}
	cur, _ := os.Getwd()
	if dir == cur {
		saveWorkspace(dir)
		return dir, nil
	}
	if err := os.Chdir(dir); err != nil {
		return "", err
	}
	// Resolve the new folder's default model from its own config.
	model := ""
	if cfg, cerr := config.Load(); cerr == nil {
		model = cfg.DefaultModel
		if e, ok := cfg.ResolveModel(cfg.DefaultModel); ok {
			model = e.Name + "/" + e.Model
		}
	}
	ctrl, err := boot.Build(a.ctx, boot.Options{
		Model: model, RequireKey: false, Sink: a.sink,
		SessionDir: config.WorkspaceSessionDir(dir),
	})
	if err != nil {
		_ = os.Chdir(cur) // roll back; the current session stays intact
		return "", err
	}
	saveWorkspace(dir) // remember it so the next launch reopens here
	// Commit the switch: save and tear down the old session, then swap in the new
	// project's controller with a fresh session file.
	a.mu.Lock()
	if a.ctrl != nil {
		_ = a.ctrl.Snapshot()
		a.ctrl.Close()
	}
	a.ctrl = ctrl
	a.model = model
	a.label = ctrl.Label()
	a.startupErr = ""
	a.mu.Unlock()
	ctrl.EnableInteractiveApproval()
	if d := ctrl.SessionDir(); d != "" {
		ctrl.SetSessionPath(agent.NewSessionPath(d, ctrl.Label()))
	}
	return dir, nil
}

// ListDir lists one directory level (directories first, then files, each
// alphabetical) for the "@" file-reference menu. rel resolves against the process
// cwd; "" lists the cwd. The menu navigates one level at a time, never
// recursively — bounded for huge trees.
func (a *App) ListDir(rel string) []DirEntry {
	base, err := os.Getwd()
	if err != nil {
		return nil
	}
	dir := base
	if rel != "" {
		if filepath.IsAbs(rel) {
			dir = filepath.Clean(rel)
		} else {
			dir = filepath.Join(base, rel)
		}
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs, files []DirEntry
	for _, e := range es {
		name := e.Name()
		if atSkip[name] {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, DirEntry{Name: name, IsDir: true})
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, DirEntry{Name: name, IsDir: false})
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })
	return append(dirs, files...)
}

// ReadFile returns a small text preview for a file under the current workspace.
func (a *App) ReadFile(rel string) FilePreview {
	out := FilePreview{Path: rel}
	path, ok, err := workspacePath(rel)
	if err != nil || !ok {
		out.Err = "invalid path"
		return out
	}
	info, err := os.Stat(path)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	if info.IsDir() {
		out.Err = "path is a directory"
		return out
	}
	if !info.Mode().IsRegular() {
		out.Err = "path is not a regular file"
		return out
	}
	out.Size = info.Size()
	f, err := os.Open(path)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	defer f.Close()

	buf := make([]byte, filePreviewLimit+1)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		out.Err = err.Error()
		return out
	}
	data := buf[:n]
	if len(data) > filePreviewLimit {
		data = data[:filePreviewLimit]
		out.Truncated = true
		data = trimUTF8PartialSuffix(data)
	}
	if bytes.Contains(data, []byte{0}) || !utf8.Valid(data) {
		out.Binary = true
		return out
	}
	out.Body = string(data)
	return out
}

// OpenWorkspacePath opens a file or folder from the workspace in the OS default app.
func (a *App) OpenWorkspacePath(rel string) error {
	path, ok, err := workspacePath(rel)
	if err != nil || !ok {
		return os.ErrInvalid
	}
	return openWorkspacePath(path)
}

// RevealWorkspacePath shows a workspace file in the native file manager.
func (a *App) RevealWorkspacePath(rel string) error {
	path, ok, err := workspacePath(rel)
	if err != nil || !ok {
		return os.ErrInvalid
	}
	switch goruntime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	case "windows":
		return exec.Command("explorer", "/select,", path).Start()
	default:
		dir := path
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			dir = filepath.Dir(path)
		}
		return exec.Command("xdg-open", dir).Start()
	}
}

// SavePastedImage stores a browser clipboard image data URL under
// .tianxuan/attachments and returns the relative @-reference path.
func (a *App) SavePastedImage(dataURL string) (string, error) {
	return control.SaveImageDataURL(dataURL)
}

// AttachmentDataURL returns a safe data URL for a stored image attachment.
func (a *App) AttachmentDataURL(path string) (string, error) {
	return control.ImageDataURL(path)
}

// WorkspaceChanges returns the files modified during the current session.
func (a *App) WorkspaceChanges() []WorkspaceChangeView {
	a.mu.RLock()
	ctrl := a.ctrl
	a.mu.RUnlock()
	if ctrl == nil {
		return nil
	}
	changes := ctrl.WorkspaceChanges()
	out := make([]WorkspaceChangeView, len(changes))
	for i, ch := range changes {
		out[i] = WorkspaceChangeView{Path: ch.Path, Added: ch.Added, Removed: ch.Removed}
	}
	return out
}
