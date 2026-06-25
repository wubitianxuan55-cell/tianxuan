package agent

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tianxuan/internal/provider"
)

type legacyEvent struct {
	Type             string           `json:"type"`
	Text             string           `json:"text"`
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoningContent"`
	ToolCalls        []legacyToolCall `json:"toolCalls"`
	CallID           string           `json:"callId"`
	Output           string           `json:"output"`
}

type legacyToolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

const legacyImportMarker = ".legacy-imported"
const legacyEventsHomeImportMarker = ".legacy-imported.v0-events-home"
const legacyEventsConfigImportMarker = ".legacy-imported.v0-events-config"
const legacyRoutedHomeImportMarker = ".legacy-imported.v2-routed"
const legacyRoutedConfigImportMarker = ".legacy-imported.v0-events-config.v2-routed"
const legacyJsonlPassMarker = ".legacy-imported.v3-jsonl"

type legacyMeta struct {
	Workspace string `json:"workspace"`
	Summary   string `json:"summary"`
}

func MigrateLegacySessions(srcDir, globalDest string, projectDir func(workspaceRoot string) string) (int, error) {
	return migrateLegacySessions(srcDir, globalDest, legacyRoutedHomeImportMarker, projectDir)
}

func MigrateLegacySessionsFromConfigDir(srcDir, globalDest string, projectDir func(workspaceRoot string) string) (int, error) {
	return migrateLegacySessions(srcDir, globalDest, legacyRoutedConfigImportMarker, projectDir)
}

func migrateLegacySessions(srcDir, globalDest, marker string, projectDir func(string) string) (int, error) {
	if strings.TrimSpace(marker) == "" {
		marker = legacyImportMarker
	}
	if importMarkerExists(globalDest, marker) && importMarkerExists(globalDest, legacyJsonlPassMarker) {
		return rehomeStrandedSessions(srcDir, globalDest, marker, projectDir)
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, nil
	}
	hasEvents := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".events.jsonl") {
			hasEvents[strings.TrimSuffix(name, ".events.jsonl")] = true
		}
	}
	imported := 0
	hadArtifactFailure := false
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".events.jsonl") {
			continue
		}
		base := strings.TrimSuffix(name, ".events.jsonl")
		meta := readLegacyMeta(srcDir, base)
		destDir := globalDest
		if projectDir != nil && meta.Workspace != "" && dirExists(meta.Workspace) {
			if d := projectDir(meta.Workspace); d != "" {
				destDir = d
			}
		}
		dest := filepath.Join(destDir, base+".jsonl")
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		eventsInfo, _ := e.Info()
		if destDir != globalDest && moveFlatImport(filepath.Join(globalDest, base+".jsonl"), dest, eventsInfo) {
			recordImportedTitle(destDir, base, meta.Summary)
			imported++
			continue
		}
		jsonlPath := filepath.Join(srcDir, base+".jsonl")
		if jsonlInfo, err := os.Stat(jsonlPath); err == nil && isMessageFormat(jsonlPath) {
			if eventsInfo == nil || !jsonlInfo.ModTime().Before(eventsInfo.ModTime()) {
				if err := transformAndCopyJsonl(jsonlPath, dest); err == nil {
					if eventsInfo != nil {
						_ = os.Chtimes(dest, eventsInfo.ModTime(), eventsInfo.ModTime())
					}
					recordImportedTitle(destDir, base, meta.Summary)
					imported++
					continue
				}
			}
		}
		msgs, err := reconstructSession(filepath.Join(srcDir, name))
		if err != nil || len(msgs) == 0 {
			continue
		}
		s := &Session{Messages: msgs}
		if err := s.Save(dest); err != nil {
			return imported, err
		}
		if eventsInfo != nil {
			_ = os.Chtimes(dest, eventsInfo.ModTime(), eventsInfo.ModTime())
		}
		recordImportedTitle(destDir, base, meta.Summary)
		imported++
	}
	if !importMarkerExists(globalDest, legacyJsonlPassMarker) {
		n, failed := importJsonlSessions(entries, srcDir, globalDest, hasEvents, projectDir)
		imported += n
		hadArtifactFailure = hadArtifactFailure || failed
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".jsonl.bak") {
				continue
			}
			base := strings.TrimSuffix(name, ".jsonl.bak")
			if hasEvents[base] {
				continue
			}
			jsonlName := base + ".jsonl"
			if _, err := os.Stat(filepath.Join(srcDir, jsonlName)); err == nil {
				continue
			}
			meta := readLegacyMeta(srcDir, base)
			destDir := globalDest
			if projectDir != nil && meta.Workspace != "" && dirExists(meta.Workspace) {
				if d := projectDir(meta.Workspace); d != "" {
					destDir = d
				}
			}
			dest := filepath.Join(destDir, base+".jsonl")
			if _, err := os.Stat(dest); err == nil {
				continue
			}
			bakPath := filepath.Join(srcDir, name)
			if !isMessageFormat(bakPath) {
				continue
			}
			srcInfo, _ := e.Info()
			if err := transformAndCopyJsonl(bakPath, dest); err != nil {
				continue
			}
			if srcInfo != nil {
				_ = os.Chtimes(dest, srcInfo.ModTime(), srcInfo.ModTime())
			}
			recordImportedTitle(destDir, base, meta.Summary)
			imported++
		}
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == "subagents" {
			continue
		}
		subDir := filepath.Join(srcDir, e.Name())
		n, _ := migrateSubDirectory(subDir, globalDest, projectDir)
		imported += n
	}
	if hadArtifactFailure {
		return imported, nil
	}
	writeImportMarkers(globalDest, marker, legacyImportMarker, legacyEventsHomeImportMarker, legacyEventsConfigImportMarker, legacyJsonlPassMarker)
	return imported, nil
}

func jsonlSessionDestDir(srcDir, srcPath, base, globalDest string, projectDir func(string) string) (string, string, bool) {
	if meta, ok, err := LoadBranchMeta(srcPath); err == nil && ok {
		summary := strings.TrimSpace(meta.TopicTitle)
		scope := meta.DefaultScope()
		if projectDir != nil && scope == "project" && meta.WorkspaceRoot != "" && dirExists(meta.WorkspaceRoot) {
			if d := projectDir(meta.WorkspaceRoot); d != "" {
				return d, summary, true
			}
		}
		if meta.Scope != "" {
			return globalDest, summary, scope == "global"
		}
	}
	meta := readLegacyMeta(srcDir, base)
	destDir := globalDest
	if projectDir != nil && meta.Workspace != "" && dirExists(meta.Workspace) {
		if d := projectDir(meta.Workspace); d != "" {
			destDir = d
		}
	}
	return destDir, meta.Summary, false
}

func migrateSubDirectory(subDir, globalDest string, projectDir func(string) string) (int, error) {
	entries, err := os.ReadDir(subDir)
	if err != nil {
		return 0, nil
	}
	hasEvents := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".events.jsonl") {
			hasEvents[strings.TrimSuffix(name, ".events.jsonl")] = true
		}
	}
	imported := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		var base string
		var srcPath string
		reconstruct := false
		switch {
		case strings.HasSuffix(name, ".events.jsonl"):
			base = strings.TrimSuffix(name, ".events.jsonl")
			srcPath = filepath.Join(subDir, name)
			if jsonlPath := filepath.Join(subDir, base+".jsonl"); fileExists(jsonlPath) && isMessageFormat(jsonlPath) {
				eventsInfo, _ := e.Info()
				if jsonlInfo, err := os.Stat(jsonlPath); err == nil {
					if eventsInfo == nil || !jsonlInfo.ModTime().Before(eventsInfo.ModTime()) {
						srcPath = jsonlPath
						reconstruct = false
					} else {
						reconstruct = true
					}
				} else {
					reconstruct = true
				}
			} else {
				reconstruct = true
			}
		case strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".events.jsonl") && !strings.HasSuffix(name, ".jsonl.bak"):
			base = strings.TrimSuffix(name, ".jsonl")
			if hasEvents[base] {
				continue
			}
			srcPath = filepath.Join(subDir, name)
			if !isMessageFormat(srcPath) {
				continue
			}
		default:
			continue
		}
		meta := readLegacyMeta(subDir, base)
		destDir := globalDest
		if projectDir != nil && meta.Workspace != "" && dirExists(meta.Workspace) {
			if d := projectDir(meta.Workspace); d != "" {
				destDir = d
			}
		}
		dest := filepath.Join(destDir, base+".jsonl")
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		srcInfo, _ := e.Info()
		if reconstruct {
			msgs, err := reconstructSession(srcPath)
			if err != nil || len(msgs) == 0 {
				continue
			}
			s := &Session{Messages: msgs}
			if err := s.Save(dest); err != nil {
				return imported, err
			}
		} else {
			if err := transformAndCopyJsonl(srcPath, dest); err != nil {
				continue
			}
		}
		if srcInfo != nil {
			_ = os.Chtimes(dest, srcInfo.ModTime(), srcInfo.ModTime())
		}
		recordImportedTitle(destDir, base, meta.Summary)
		imported++
	}
	return imported, nil
}

func isMessageFormat(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var buf [64]byte
	n, _ := f.Read(buf[:])
	s := strings.TrimLeft(string(buf[:n]), " \t\r\n")
	return strings.HasPrefix(s, `{"role":`)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type legacyAssistantMsg struct {
	Role      string          `json:"role"`
	ToolCalls json.RawMessage `json:"tool_calls"`
}

type legacyToolCallObj struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func transformAndCopyJsonl(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".session.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmpPath)
		}
	}()
	enc := json.NewEncoder(tmp)
	dec := json.NewDecoder(in)
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			break
		}
		var m legacyAssistantMsg
		if err := json.Unmarshal(raw, &m); err != nil || m.Role != "assistant" || len(m.ToolCalls) == 0 {
			if err := enc.Encode(raw); err != nil {
				return err
			}
			continue
		}
		var legacyCalls []legacyToolCallObj
		if err := json.Unmarshal(m.ToolCalls, &legacyCalls); err != nil || len(legacyCalls) == 0 {
			if err := enc.Encode(raw); err != nil {
				return err
			}
			continue
		}
		flatCalls := make([]provider.ToolCall, len(legacyCalls))
		for i, tc := range legacyCalls {
			flatCalls[i] = provider.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
		var full map[string]json.RawMessage
		if err := json.Unmarshal(raw, &full); err != nil {
			if err := enc.Encode(raw); err != nil {
				return err
			}
			continue
		}
		b, err := json.Marshal(flatCalls)
		if err != nil {
			if err := enc.Encode(raw); err != nil {
				return err
			}
			continue
		}
		full["tool_calls"] = b
		if err := enc.Encode(full); err != nil {
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return err
	}
	ok = true
	return nil
}

func readLegacyMeta(srcDir, base string) legacyMeta {
	var m legacyMeta
	b, err := os.ReadFile(filepath.Join(srcDir, base+".meta.json"))
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	m.Workspace = strings.TrimSpace(m.Workspace)
	m.Summary = strings.TrimSpace(m.Summary)
	return m
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func moveFlatImport(oldPath, newPath string, srcInfo os.FileInfo) bool {
	if srcInfo == nil {
		return false
	}
	info, err := os.Stat(oldPath)
	if err != nil {
		return false
	}
	d := info.ModTime().Sub(srcInfo.ModTime())
	if d < -2*time.Second || d > 2*time.Second {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return false
	}
	return os.Rename(oldPath, newPath) == nil
}

func recordImportedTitle(destDir, base, summary string) {
	if summary == "" {
		return
	}
	path := filepath.Join(destDir, ".titles.json")
	titles := map[string]string{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &titles)
	}
	key := base + ".jsonl"
	if titles[key] != "" {
		return
	}
	titles[key] = summary
	b, err := json.MarshalIndent(titles, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func importMarkerExists(destDir, marker string) bool {
	if strings.TrimSpace(destDir) == "" || strings.TrimSpace(marker) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(destDir, marker))
	return err == nil
}

func writeImportMarkers(destDir string, markers ...string) {
	if strings.TrimSpace(destDir) == "" {
		return
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return
	}
	seen := map[string]bool{}
	for _, marker := range markers {
		marker = strings.TrimSpace(marker)
		if marker == "" || seen[marker] {
			continue
		}
		seen[marker] = true
		_ = os.WriteFile(filepath.Join(destDir, marker), nil, 0o644)
	}
}

func rehomeStrandedSessions(srcDir, globalDest, marker string, projectDir func(string) string) (int, error) {
	if projectDir == nil {
		return 0, nil
	}
	markerPath := filepath.Join(globalDest, marker)
	markerInfo, err := os.Stat(markerPath)
	if err != nil {
		return 0, nil
	}
	watermark := markerInfo.ModTime()
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, nil
	}
	imported := 0
	hadCopyFailure := false
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jsonl") ||
			strings.HasSuffix(name, ".events.jsonl") || strings.HasSuffix(name, ".jsonl.bak") {
			continue
		}
		base := strings.TrimSuffix(name, ".jsonl")
		if strings.HasPrefix(base, "subagent-") {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil || !info.ModTime().After(watermark) {
			continue
		}
		srcPath := filepath.Join(srcDir, name)
		if !isMessageFormat(srcPath) {
			continue
		}
		destDir, summary := strandedSessionDestDir(srcDir, srcPath, base, projectDir)
		if destDir == "" || sameDirPath(destDir, globalDest) {
			continue
		}
		dest := filepath.Join(destDir, name)
		if _, err := os.Stat(dest); err == nil {
			_ = copySubagentArtifacts(srcDir, destDir, base)
			continue
		}
		if err := transformAndCopyJsonl(srcPath, dest); err != nil {
			hadCopyFailure = true
			continue
		}
		_ = os.Chtimes(dest, info.ModTime(), info.ModTime())
		copyBranchMetaSidecar(srcPath, dest)
		_ = copySubagentArtifacts(srcDir, destDir, base)
		recordImportedTitle(destDir, base, summary)
		imported++
	}
	if !hadCopyFailure {
		now := time.Now()
		_ = os.Chtimes(markerPath, now, now)
	}
	return imported, nil
}

func strandedSessionDestDir(srcDir, srcPath, base string, projectDir func(string) string) (string, string) {
	if meta, ok, err := LoadBranchMeta(srcPath); err == nil && ok {
		if meta.DefaultScope() == "project" && meta.WorkspaceRoot != "" && dirExists(meta.WorkspaceRoot) {
			if d := projectDir(meta.WorkspaceRoot); d != "" {
				return d, strings.TrimSpace(meta.TopicTitle)
			}
		}
		if meta.Scope != "" {
			return "", ""
		}
	}
	legacy := readLegacyMeta(srcDir, base)
	if legacy.Workspace != "" && dirExists(legacy.Workspace) {
		if d := projectDir(legacy.Workspace); d != "" {
			return d, legacy.Summary
		}
	}
	return "", ""
}

func copyBranchMetaSidecar(srcPath, dstPath string) {
	b, err := os.ReadFile(BranchMetaPath(srcPath))
	if err != nil {
		return
	}
	dstMeta := BranchMetaPath(dstPath)
	if err := os.MkdirAll(filepath.Dir(dstMeta), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(dstMeta), ".branch.*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, dstMeta); err != nil {
		os.Remove(tmpPath)
	}
}

func copySubagentArtifacts(srcSessionDir, dstSessionDir, parentSession string) error {
	if sameDirPath(srcSessionDir, dstSessionDir) {
		return nil
	}
	artifacts, err := ListSubagentsByParent(srcSessionDir, parentSession)
	if err != nil {
		return err
	}
	var errs []error
	dstSubagentDir := filepath.Join(dstSessionDir, "subagents")
	for _, artifact := range artifacts {
		for _, src := range []string{artifact.SessionPath, artifact.MetaPath} {
			if err := copyFileIfExists(src, filepath.Join(dstSubagentDir, filepath.Base(src))); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func copyFileIfExists(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".subagent.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return err
	}
	_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
	return nil
}

func sameDirPath(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if ca == cb {
		return true
	}
	if aa, err := filepath.Abs(ca); err == nil {
		if bb, err := filepath.Abs(cb); err == nil {
			return aa == bb
		}
	}
	return false
}

func reconstructSession(path string) ([]provider.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var msgs []provider.Message
	toolName := map[string]string{}
	dec := json.NewDecoder(f)
	for {
		var e legacyEvent
		if err := dec.Decode(&e); err != nil {
			if !errors.Is(err, io.EOF) {
				return msgs, nil
			}
			break
		}
		switch e.Type {
		case "user.message":
			if e.Text != "" {
				msgs = append(msgs, provider.Message{Role: provider.RoleUser, Content: e.Text})
			}
		case "model.final":
			m := provider.Message{Role: provider.RoleAssistant, Content: e.Content, ReasoningContent: e.ReasoningContent}
			for _, tc := range e.ToolCalls {
				m.ToolCalls = append(m.ToolCalls, provider.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
				toolName[tc.ID] = tc.Function.Name
			}
			msgs = append(msgs, m)
		case "tool.result":
			msgs = append(msgs, provider.Message{Role: provider.RoleTool, ToolCallID: e.CallID, Name: toolName[e.CallID], Content: e.Output})
		}
	}
	return msgs, nil
}

func importJsonlSessions(entries []os.DirEntry, srcDir, globalDest string, hasEvents map[string]bool, projectDir func(string) string) (int, bool) {
	imported := 0
	hadArtifactFailure := false
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".events.jsonl") || strings.HasSuffix(name, ".jsonl.bak") {
			continue
		}
		base := strings.TrimSuffix(name, ".jsonl")
		if hasEvents[base] {
			continue
		}
		if strings.HasPrefix(base, "subagent-") {
			continue
		}
		jsonlPath := filepath.Join(srcDir, name)
		if !isMessageFormat(jsonlPath) {
			continue
		}
		destDir, summary, copyBranchMeta := jsonlSessionDestDir(srcDir, jsonlPath, base, globalDest, projectDir)
		dest := filepath.Join(destDir, base+".jsonl")
		if _, err := os.Stat(dest); err == nil {
			if copyBranchMeta {
				if err := copySubagentArtifacts(srcDir, destDir, base); err != nil {
					hadArtifactFailure = true
				}
			}
			continue
		}
		srcInfo, _ := e.Info()
		if err := transformAndCopyJsonl(jsonlPath, dest); err != nil {
			continue
		}
		if srcInfo != nil {
			_ = os.Chtimes(dest, srcInfo.ModTime(), srcInfo.ModTime())
		}
		if copyBranchMeta {
			copyBranchMetaSidecar(jsonlPath, dest)
			if err := copySubagentArtifacts(srcDir, destDir, base); err != nil {
				hadArtifactFailure = true
			}
		}
		recordImportedTitle(destDir, base, summary)
		imported++
	}
	return imported, hadArtifactFailure
}
