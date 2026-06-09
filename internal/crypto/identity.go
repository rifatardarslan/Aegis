package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// Identity represents the ephemeral application-level Ed25519 keypair.
type Identity struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// NewIdentity generates a fresh ephemeral Ed25519 keypair for the session.
func NewIdentity() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate identity keypair: %w", err)
	}
	return &Identity{
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
}

// Fingerprint returns the SHA-256 base64 fingerprint of the public key (SSH style).
func Fingerprint(pubKey ed25519.PublicKey) string {
	sum := sha256.Sum256(pubKey)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// SAS computes a 3-word Short Authentication String (SAS) from the XOR of both public keys.
func SAS(localPub, remotePub ed25519.PublicKey) string {
	localHash := sha256.Sum256(localPub)
	remoteHash := sha256.Sum256(remotePub)

	// XOR the first 3 bytes of the hashes to derive indices [0, 255]
	b1 := localHash[0] ^ remoteHash[0]
	b2 := localHash[1] ^ remoteHash[1]
	b3 := localHash[2] ^ remoteHash[2]

	w1 := BIP39Words[int(b1)]
	w2 := BIP39Words[int(b2)]
	w3 := BIP39Words[int(b3)]

	return fmt.Sprintf("%s  ·  %s  ·  %s", strings.ToUpper(w1), strings.ToUpper(w2), strings.ToUpper(w3))
}

// Destroy zeroes out the identity private key.
func (id *Identity) Destroy() {
	if id.PrivateKey != nil {
		Bytes(id.PrivateKey)
	}
}
