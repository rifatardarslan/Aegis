package transfer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrExists = errors.New("destination exists; confirm overwrite")

func SaveFile(path string, data []byte) error { return SaveFileWithOverwrite(path, data, false) }

// SaveFileWithOverwrite writes a private staging file and publishes it only when
// complete. Existing paths are never followed as symlinks or truncated in place.
func SaveFileWithOverwrite(path string, data []byte, overwrite bool) error {
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil {
		if !overwrite {
			return ErrExists
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("destination must be a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".aegis-save-*")
	if err != nil {
		return err
	}
	staging := f.Name()
	defer os.Remove(staging)
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if !overwrite {
		// Linking is atomic and fails if another process created the destination.
		if err := os.Link(staging, path); err != nil {
			if os.IsExist(err) {
				return ErrExists
			}
			return fmt.Errorf("publish received file: %w", err)
		}
		return nil
	}
	if err := os.Rename(staging, path); err != nil {
		return fmt.Errorf("replace received file: %w", err)
	}
	return nil
}
