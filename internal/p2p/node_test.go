package p2p

import (
	"bytes"
	"testing"
)

func TestDeriveHostKey(t *testing.T) {
	// Use separate string constructions to avoid any shared-backing-array effects
	// from unsafe zeroization of Go strings elsewhere in the codebase.
	key1, err := DeriveHostKey(string([]byte("super-secure-passphrase-123")))
	if err != nil {
		t.Fatalf("failed to derive key 1: %v", err)
	}

	key2, err := DeriveHostKey(string([]byte("super-secure-passphrase-123")))
	if err != nil {
		t.Fatalf("failed to derive key 2: %v", err)
	}

	raw1, err := key1.Raw()
	if err != nil {
		t.Fatalf("failed to get raw key 1: %v", err)
	}

	raw2, err := key2.Raw()
	if err != nil {
		t.Fatalf("failed to get raw key 2: %v", err)
	}

	if !bytes.Equal(raw1, raw2) {
		t.Fatal("derived keys from identical passphrases are not equal")
	}

	key3, err := DeriveHostKey(string([]byte("super-secure-passphrase-123-different")))
	if err != nil {
		t.Fatalf("failed to derive key 3: %v", err)
	}

	raw3, err := key3.Raw()
	if err != nil {
		t.Fatalf("failed to get raw key 3: %v", err)
	}

	if bytes.Equal(raw1, raw3) {
		t.Fatal("derived keys from different passphrases are equal")
	}
}

func TestStreamFraming(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("hello-aegis-secure-p2p")

	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatalf("failed to write frame: %v", err)
	}

	readPayload, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("failed to read frame: %v", err)
	}

	if !bytes.Equal(payload, readPayload) {
		t.Fatalf("expected payload %q, got %q", payload, readPayload)
	}
}
