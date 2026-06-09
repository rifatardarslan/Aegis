package crypto

import (
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// Seal encrypts plaintext using ChaCha20-Poly1305 AEAD.
func Seal(key [32]byte, nonce [12]byte, plaintext, aad []byte) ([]byte, error) {
	defer Array32(&key)
	defer Bytes(nonce[:])
	aead, err := chacha20poly1305.New(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create chacha20poly1305 cipher: %w", err)
	}
	return aead.Seal(nil, nonce[:], plaintext, aad), nil
}

// Open decrypts and authenticates ciphertext using ChaCha20-Poly1305 AEAD.
func Open(key [32]byte, nonce [12]byte, ciphertext, aad []byte) ([]byte, error) {
	defer Array32(&key)
	defer Bytes(nonce[:])
	aead, err := chacha20poly1305.New(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create chacha20poly1305 cipher: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce[:], ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("message decryption or authentication failed: %w", err)
	}
	return plaintext, nil
}
