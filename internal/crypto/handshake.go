package crypto

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/hkdf"
	"io"
)

type HandshakeResult struct {
	SharedRootKey  [32]byte
	LocalIdentity  *Identity
	RemotePubKey   ed25519.PublicKey
	TranscriptHash [32]byte
}

func RunHandshake(rw io.ReadWriter, id *Identity, initiator bool) (*HandshakeResult, error) {
	return RunBoundHandshake(rw, id, initiator, nil)
}

// RunBoundHandshake authenticates ephemeral keys and binds them to the request
// and transport peer identities. It does not authenticate a person's real name.
func RunBoundHandshake(rw io.ReadWriter, id *Identity, initiator bool, context []byte) (*HandshakeResult, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	local := make([]byte, 64)
	copy(local, priv.PublicKey().Bytes())
	copy(local[32:], id.PublicKey)
	remote := make([]byte, 64)
	exchange := func(out, in []byte) error {
		if initiator {
			if err := writeFull(rw, out); err != nil {
				return err
			}
			_, err := io.ReadFull(rw, in)
			return err
		}
		if _, err := io.ReadFull(rw, in); err != nil {
			return err
		}
		return writeFull(rw, out)
	}
	if err := exchange(local, remote); err != nil {
		return nil, fmt.Errorf("exchange public keys: %w", err)
	}
	transcript := append([]byte("aegis-handshake-v2\x00"), context...)
	if initiator {
		transcript = append(transcript, local...)
		transcript = append(transcript, remote...)
	} else {
		transcript = append(transcript, remote...)
		transcript = append(transcript, local...)
	}
	digest := sha256.Sum256(transcript)
	sig := ed25519.Sign(id.PrivateKey, digest[:])
	defer Bytes(sig)
	remoteSig := make([]byte, ed25519.SignatureSize)
	if err := exchange(sig, remoteSig); err != nil {
		return nil, fmt.Errorf("exchange signatures: %w", err)
	}
	pub := ed25519.PublicKey(append([]byte(nil), remote[32:]...))
	if !ed25519.Verify(pub, digest[:], remoteSig) {
		return nil, fmt.Errorf("invalid handshake signature")
	}
	remoteDH, err := ecdh.X25519().NewPublicKey(remote[:32])
	if err != nil {
		return nil, err
	}
	secret, err := priv.ECDH(remoteDH)
	if err != nil {
		return nil, err
	}
	defer Bytes(secret)
	var root [32]byte
	if _, err := io.ReadFull(hkdf.New(sha256.New, secret, digest[:], []byte("aegis-root-v2")), root[:]); err != nil {
		return nil, err
	}
	return &HandshakeResult{SharedRootKey: root, LocalIdentity: id, RemotePubKey: pub, TranscriptHash: digest}, nil
}

func writeFull(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(b) {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

// TranscriptSAS provides 48 bits for optional independent comparison.
func TranscriptSAS(hash [32]byte) string {
	return fmt.Sprintf("%s · %s · %s · %s · %s · %s", BIP39Words[hash[0]], BIP39Words[hash[1]], BIP39Words[hash[2]], BIP39Words[hash[3]], BIP39Words[hash[4]], BIP39Words[hash[5]])
}
