// Package fileutil provides file system utilities shared across the kernel.
package fileutil

import (
	"os"
	"path/filepath"
)

// ReplaceFile atomically replaces the file at path with the given content by
// writing to a temporary sibling and renaming it over the target. This prevents
// readers from seeing partial writes and guards against crash-induced corruption.
func ReplaceFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
