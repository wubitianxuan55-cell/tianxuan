package environment

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const probeSnapshotTTL = 24 * time.Hour

const probeSnapshotVersion = 1

type probeSnapshot struct {
	Version     int           `json:"version"`
	Fingerprint string        `json:"fingerprint"`
	StoredAt    time.Time     `json:"stored_at"`
	Results     []ProbeResult `json:"results"`
}

func probeSnapshotPath(dir, fingerprint string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(fingerprint))
	return filepath.Join(dir, "environment", fmt.Sprintf("probes-%x.json", sum[:8]))
}

func loadProbeSnapshot(dir, fingerprint string) (probeSnapshot, bool) {
	path := probeSnapshotPath(dir, fingerprint)
	if path == "" {
		return probeSnapshot{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return probeSnapshot{}, false
	}
	var snap probeSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return probeSnapshot{}, false
	}
	if snap.Version != probeSnapshotVersion || snap.Fingerprint != fingerprint {
		return probeSnapshot{}, false
	}
	return snap, true
}

func saveProbeSnapshot(dir, fingerprint string, results []ProbeResult, now time.Time) {
	path := probeSnapshotPath(dir, fingerprint)
	if path == "" {
		return
	}
	b, err := json.Marshal(probeSnapshot{
		Version:     probeSnapshotVersion,
		Fingerprint: fingerprint,
		StoredAt:    now,
		Results:     results,
	})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".probes.*.tmp")
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
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
	}
}

func transientProbeFailure(r ProbeResult) bool {
	if r.Found {
		return false
	}
	return r.Error == "timeout" || strings.HasPrefix(r.Error, "exit ")
}

func mergeProbeSnapshot(previous, fresh []ProbeResult) []ProbeResult {
	if len(previous) == 0 {
		return fresh
	}
	prevByCommand := make(map[string]ProbeResult, len(previous))
	for _, r := range previous {
		if r.Found {
			prevByCommand[r.Command] = r
		}
	}
	merged := append([]ProbeResult(nil), fresh...)
	for i, r := range merged {
		if !transientProbeFailure(r) {
			continue
		}
		if prev, ok := prevByCommand[r.Command]; ok {
			merged[i] = prev
		}
	}
	sortResults(merged)
	return merged
}
