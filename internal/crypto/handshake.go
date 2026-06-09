package crypto

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// HandshakeResult encapsulates the derived session keys and identities.
type HandshakeResult struct {
	SharedRootKey [32]byte
	LocalIdentity *Identity
	RemotePubKey  ed25519.PublicKey
}

// RunHandshake performs an ephemeral X25519 DH exchange and Ed25519 identity authentication
// over the raw P2P network reader and writer.
func RunHandshake(rw io.ReadWriter, localIdent *Identity, isInitiator bool) (*HandshakeResult, error) {
	// 1. Generate ephemeral X25519 keypair
	ephemPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral X25519 key: %w", err)
	}
	localX509Pub := ephemPriv.PublicKey().Bytes()

	// 2. Exchange ephemeral public keys
	var remoteX509Pub []byte
	if isInitiator {
		// Initiator writes first
		if _, err := rw.Write(localX509Pub); err != nil {
			return nil, fmt.Errorf("initiator failed to write ephemeral pubkey: %w", err)
		}
		remoteX509Pub = make([]byte, 32)
		if _, err := io.ReadFull(rw, remoteX509Pub); err != nil {
			return nil, fmt.Errorf("initiator failed to read ephemeral pubkey: %w", err)
		}
	} else {
		// Responder reads first
		remoteX509Pub = make([]byte, 32)
		if _, err := io.ReadFull(rw, remoteX509Pub); err != nil {
			return nil, fmt.Errorf("responder failed to read ephemeral pubkey: %w", err)
		}
		if _, err := rw.Write(localX509Pub); err != nil {
			return nil, fmt.Errorf("responder failed to write ephemeral pubkey: %w", err)
		}
	}

	// 3. Compute X25519 shared secret
	remotePub, err := ecdh.X25519().NewPublicKey(remoteX509Pub)
	if err != nil {
		return nil, fmt.Errorf("invalid remote ephemeral pubkey: %w", err)
	}
	sharedSecret, err := ephemPriv.ECDH(remotePub)
	if err != nil {
		return nil, fmt.Errorf("X25519 key exchange failed: %w", err)
	}
	defer Bytes(sharedSecret)

	// 4. Construct verification transcript to sign
	// E.g. transcript = initiator_X25519 || responder_X25519
	var transcript []byte
	if isInitiator {
		transcript = append(localX509Pub, remoteX509Pub...)
	} else {
		transcript = append(remoteX509Pub, localX509Pub...)
	}

	// 5. Sign the transcript using our Ed25519 identity key
	localSig := ed25519.Sign(localIdent.PrivateKey, transcript)
	defer Bytes(localSig)

	// 6. Pack identity verification payload: [32B Ed25519 public key][64B transcript signature]
	localVerifyPayload := make([]byte, 32+64)
	defer Bytes(localVerifyPayload)
	copy(localVerifyPayload[:32], localIdent.PublicKey)
	copy(localVerifyPayload[32:], localSig)

	// 7. Exchange identity payload
	var remoteVerifyPayload []byte
	if isInitiator {
		if _, err := rw.Write(localVerifyPayload); err != nil {
			return nil, fmt.Errorf("failed to send local identity: %w", err)
		}
		remoteVerifyPayload = make([]byte, 32+64)
		if _, err := io.ReadFull(rw, remoteVerifyPayload); err != nil {
			return nil, fmt.Errorf("failed to receive remote identity: %w", err)
		}
	} else {
		remoteVerifyPayload = make([]byte, 32+64)
		if _, err := io.ReadFull(rw, remoteVerifyPayload); err != nil {
			return nil, fmt.Errorf("failed to receive remote identity: %w", err)
		}
		if _, err := rw.Write(localVerifyPayload); err != nil {
			return nil, fmt.Errorf("failed to send local identity: %w", err)
		}
	}

	remoteIdentPub := ed25519.PublicKey(remoteVerifyPayload[:32])
	remoteSig := remoteVerifyPayload[32:]

	// 8. Verify remote peer's signature against transcript
	if !ed25519.Verify(remoteIdentPub, transcript, remoteSig) {
		return nil, fmt.Errorf("man-in-the-middle detected: invalid remote identity signature")
	}

	// 9. Derive the shared root key using HKDF-SHA256
	hkdfSalt := []byte("aegis-hkdf-salt-v1")
	hkdfReader := hkdf.New(sha256.New, sharedSecret, hkdfSalt, []byte("aegis-root-v1"))
	var rootKey [32]byte
	if _, err := io.ReadFull(hkdfReader, rootKey[:]); err != nil {
		return nil, fmt.Errorf("failed to derive root key using HKDF: %w", err)
	}

	return &HandshakeResult{
		SharedRootKey: rootKey,
		LocalIdentity: localIdent,
		RemotePubKey:  remoteIdentPub,
	}, nil
}
