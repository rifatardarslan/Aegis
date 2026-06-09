package crypto

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// RatchetState holds the keys for the symmetric double ratchet session.
type RatchetState struct {
	SendChainKey [32]byte
	RecvChainKey [32]byte
	SendMsgNum   uint64
	RecvMsgNum   uint64
}

// NewRatchetState initializes the ratchet state from a shared root key.
func NewRatchetState(sharedRootKey [32]byte, isInitiator bool) *RatchetState {
	defer Array32(&sharedRootKey)
	r := hkdf.New(sha256.New, sharedRootKey[:], nil, []byte("aegis-ratchet-init-v1"))
	var keys [64]byte
	if _, err := io.ReadFull(r, keys[:]); err != nil {
		// This should never fail with HKDF, but handle defensively
		panic("aegis: HKDF key derivation failed: " + err.Error())
	}
	defer Bytes(keys[:])

	var sendKey, recvKey [32]byte
	// NOTE: Do NOT defer-zero sendKey/recvKey; they are copied by value into the struct.

	if isInitiator {
		copy(sendKey[:], keys[:32])
		copy(recvKey[:], keys[32:])
	} else {
		copy(recvKey[:], keys[:32])
		copy(sendKey[:], keys[32:])
	}

	return &RatchetState{
		// RootKey is intentionally NOT stored: it was consumed during initialization.
		// Keeping it in memory would be an unnecessary security risk.
		SendChainKey: sendKey,
		RecvChainKey: recvKey,
		SendMsgNum:   0,
		RecvMsgNum:   0,
	}
}

func kdfStep(chainKey [32]byte) ([32]byte, [32]byte, error) {
	defer Array32(&chainKey)
	r := hkdf.New(sha256.New, chainKey[:], nil, []byte("aegis-ratchet-step-v1"))
	var out [64]byte
	if _, err := io.ReadFull(r, out[:]); err != nil {
		return [32]byte{}, [32]byte{}, err
	}
	defer Bytes(out[:])

	var nextChainKey, msgKey [32]byte
	// NOTE: Do NOT defer-zero nextChainKey/msgKey here; they are returned by value.
	// Callers are responsible for zeroizing their copies after use.

	copy(nextChainKey[:], out[:32])
	copy(msgKey[:], out[32:])
	return nextChainKey, msgKey, nil
}

func makeNonce(msgNum uint64) [12]byte {
	var nonce [12]byte
	binary.BigEndian.PutUint64(nonce[:8], msgNum)
	return nonce
}

// EncryptMessage encrypts plaintext and advances the Send chain key.
func (r *RatchetState) EncryptMessage(plaintext []byte, aad []byte) ([]byte, error) {
	nextChain, msgKey, err := kdfStep(r.SendChainKey)
	if err != nil {
		return nil, fmt.Errorf("ratchet KDF advance failed: %w", err)
	}
	defer Array32(&msgKey)
	r.SendChainKey = nextChain

	nonce := makeNonce(r.SendMsgNum)
	ciphertext, err := Seal(msgKey, nonce, plaintext, aad)
	if err != nil {
		return nil, err
	}

	// Pack: [8B msgNum][ciphertext]
	payload := make([]byte, 8+len(ciphertext))
	binary.BigEndian.PutUint64(payload[:8], r.SendMsgNum)
	copy(payload[8:], ciphertext)

	r.SendMsgNum++
	return payload, nil
}

// DecryptMessage decrypts ciphertext and advances the Recv chain key.
func (r *RatchetState) DecryptMessage(payload []byte, aad []byte) ([]byte, error) {
	if len(payload) < 8 {
		return nil, errors.New("encrypted payload too short")
	}

	msgNum := binary.BigEndian.Uint64(payload[:8])
	ciphertext := payload[8:]

	if msgNum != r.RecvMsgNum {
		return nil, fmt.Errorf("replayed or out-of-order message: expected msg %d, got %d", r.RecvMsgNum, msgNum)
	}

	nextChain, msgKey, err := kdfStep(r.RecvChainKey)
	if err != nil {
		return nil, fmt.Errorf("ratchet KDF advance failed: %w", err)
	}
	defer Array32(&msgKey)
	r.RecvChainKey = nextChain

	nonce := makeNonce(msgNum)
	plaintext, err := Open(msgKey, nonce, ciphertext, aad)
	if err != nil {
		return nil, err
	}

	r.RecvMsgNum++
	return plaintext, nil
}

// Destroy zeroizes all keys in the ratchet state.
func (r *RatchetState) Destroy() {
	Array32(&r.SendChainKey)
	Array32(&r.RecvChainKey)
	r.SendMsgNum = 0
	r.RecvMsgNum = 0
}
