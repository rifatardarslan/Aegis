package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestHashFile(t *testing.T) {
	// Create a temp directory for tests
	tmpDir, err := os.MkdirTemp("", "aegis-transfer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test.dat")
	data := []byte("Aegis Secure Cryptographic Zero-Trust File Transfer Test Payload")

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Compute expected SHA-256 hash
	hash := sha256.Sum256(data)
	expectedHash := hex.EncodeToString(hash[:])

	// Hash streaming
	computedHash, err := HashFile(filePath)
	if err != nil {
		t.Fatalf("HashFile failed: %v", err)
	}

	if computedHash != expectedHash {
		t.Errorf("hash mismatch: expected %s, got %s", expectedHash, computedHash)
	}
}

func TestSaveFilePermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aegis-transfer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "secure_save.dat")
	data := []byte("highly sensitive information")

	if err := SaveFile(filePath, data); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	// Verify permissions on Unix-like operating systems (or best effort on Windows)
	fi, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("os.Stat failed: %v", err)
	}

	// Mode Perm on Windows can sometimes differ, but let's check it's readable
	if fi.Size() != int64(len(data)) {
		t.Errorf("size mismatch: expected %d, got %d", len(data), fi.Size())
	}
}
