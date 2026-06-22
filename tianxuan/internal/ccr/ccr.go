// Package ccr implements Compress-Cache-Retrieve: a local key-value store
// for original content that was compressed before being sent to the LLM.
// The agent stores originals here; the `retrieve` tool reads them back.
//
// V9.1: inspired by Headroom's CCR (Compress-Cache-Retrieve) —
// reversible compression with local file-backed storage.
package ccr

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// Write stores content and returns a retrieval key (8-char hex hash).
// Returns "" if content is empty.
func Write(content string) (key string) {
	if content == "" {
		return ""
	}
	h := sha256.Sum256([]byte(content))
	key = hex.EncodeToString(h[:8])
	_ = os.WriteFile(filepath.Join(storeDir, key+".txt"), []byte(content), 0o644)
	return key
}

// Read retrieves original content by key. Returns "" if not found.
func Read(key string) string {
	b, err := os.ReadFile(filepath.Join(storeDir, key+".txt"))
	if err != nil {
		return ""
	}
	return string(b)
}

// Summary returns a short preview of the content for a key.
func Summary(key string) string {
	v := Read(key)
	if v == "" {
		return "(not found)"
	}
	r := []rune(v)
	if len(r) > 120 {
		return string(r[:120]) + "..."
	}
	return v
}

// --- store directory ---

var storeDir string

// SetDir configures the CCR storage directory. Call once from boot.
func SetDir(dir string) {
	storeDir = dir
	_ = os.MkdirAll(dir, 0o755)
}

// Dir returns the current storage directory.
func Dir() string { return storeDir }
