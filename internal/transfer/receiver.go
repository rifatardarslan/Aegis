package transfer

import (
	"fmt"
	"os"
)

// SaveFile writes the in-memory verified buffer to disk with secure 0600 (owner-only) permissions.
func SaveFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open output file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write buffer to file: %w", err)
	}
	return nil
}
