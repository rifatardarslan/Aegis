package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
		return nil, err
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

// Destroy zeroes out the identity private key.
func (id *Identity) Destroy() {
	if id.PrivateKey != nil {
		Bytes(id.PrivateKey)
	}
}
