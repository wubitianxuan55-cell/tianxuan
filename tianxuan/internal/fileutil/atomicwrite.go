package fileutil

import "os"

// AtomicWriteFile atomically writes data to path by writing to a temp file first,
// then renaming. This prevents partial writes.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
