package shared

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data via temp file in the same directory and then
// swaps it into place to reduce partial-read windows for concurrent readers.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-write-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	defer cleanup()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		// Windows may reject rename-over-existing file.
		_ = os.Remove(path)
		if err2 := os.Rename(tmpPath, path); err2 != nil {
			return fmt.Errorf("replace file: %w", err2)
		}
	}
	return nil
}
