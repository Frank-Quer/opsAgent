package app

import (
	"os"
	"path/filepath"
)

// writeAtomic replaces a file with mode 0600, removing the temporary file on failure.
// The caller owns parent directory creation and commits memory only after success.
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".opsagent-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
