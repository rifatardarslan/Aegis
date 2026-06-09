package crypto

import (
	"bytes"
	"net"
	"sync"
	"testing"
)

func TestZeroize(t *testing.T) {
	b := []byte{1, 2, 3, 4, 5}
	Bytes(b)
	for i, val := range b {
		if val != 0 {
			t.Fatalf("byte at index %d is not zero: %d", i, val)
		}
	}

	arr := [32]byte{1, 2, 3, 4, 5}
	Array32(&arr)
	for i, val := range arr {
		if val != 0 {
			t.Fatalf("array byte at index %d is not zero: %d", i, val)
		}
	}

	s := string([]byte("secret-string"))
	String(&s)
	if s != "" {
		t.Fatalf("string was not zeroed: %q", s)
	}
}

func TestHandshakeAndRatchet(t *testing.T) {
	// Create local identities
	identA, err := NewIdentity()
	if err != nil {
		t.Fatalf("failed to create identity A: %v", err)
	}
	defer identA.Destroy()

	identB, err := NewIdentity()
	if err != nil {
		t.Fatalf("failed to create identity B: %v", err)
	}
	defer identB.Destroy()

	// In-memory pipe for local network emulation
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	var resultA, resultB *HandshakeResult
	var errA, errB error

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		resultA, errA = RunHandshake(c1, identA, true)
	}()

	go func() {
		defer wg.Done()
		resultB, errB = RunHandshake(c2, identB, false)
	}()

	wg.Wait()

	if errA != nil {
		t.Fatalf("handshake A failed: %v", errA)
	}
	if errB != nil {
		t.Fatalf("handshake B failed: %v", errB)
	}

	if !bytes.Equal(resultA.SharedRootKey[:], resultB.SharedRootKey[:]) {
		t.Fatal("shared root keys do not match")
	}

	if !bytes.Equal(resultA.RemotePubKey, identB.PublicKey) {
		t.Fatal("A's recorded remote public key is incorrect")
	}

	if !bytes.Equal(resultB.RemotePubKey, identA.PublicKey) {
		t.Fatal("B's recorded remote public key is incorrect")
	}

	// Verify Fingerprint and SAS functions
	fpA := Fingerprint(identA.PublicKey)
	fpB := Fingerprint(identB.PublicKey)
	if fpA == "" || fpB == "" {
		t.Fatal("fingerprints should not be empty")
	}

	sas1 := SAS(identA.PublicKey, identB.PublicKey)
	sas2 := SAS(identB.PublicKey, identA.PublicKey)
	if sas1 != sas2 {
		t.Fatalf("SAS is not symmetric: %q vs %q", sas1, sas2)
	}

	// ── Test Ratchet State ───────────────────────────────────────────────────
	ratchetA := NewRatchetState(resultA.SharedRootKey, true)
	defer ratchetA.Destroy()

	ratchetB := NewRatchetState(resultB.SharedRootKey, false)
	defer ratchetB.Destroy()

	aad := []byte("aegis-test-metadata-context")

	// Message 1: A -> B
	plaintext1 := []byte("secret transmission from A to B")
	ciphertext1, err := ratchetA.EncryptMessage(plaintext1, aad)
	if err != nil {
		t.Fatalf("A failed to encrypt: %v", err)
	}

	decrypted1, err := ratchetB.DecryptMessage(ciphertext1, aad)
	if err != nil {
		t.Fatalf("B failed to decrypt: %v", err)
	}

	if !bytes.Equal(plaintext1, decrypted1) {
		t.Fatalf("decrypted text does not match plaintext: expected %q, got %q", plaintext1, decrypted1)
	}

	// Message 2: B -> A
	plaintext2 := []byte("reply transmission from B to A")
	ciphertext2, err := ratchetB.EncryptMessage(plaintext2, aad)
	if err != nil {
		t.Fatalf("B failed to encrypt reply: %v", err)
	}

	decrypted2, err := ratchetA.DecryptMessage(ciphertext2, aad)
	if err != nil {
		t.Fatalf("A failed to decrypt reply: %v", err)
	}

	if !bytes.Equal(plaintext2, decrypted2) {
		t.Fatalf("decrypted reply does not match: expected %q, got %q", plaintext2, decrypted2)
	}

	// Replay attack / tampering test (Must fail because of msgNum checks or AEAD)
	_, err = ratchetA.DecryptMessage(ciphertext2, aad)
	if err == nil {
		t.Fatal("expected decryption of replayed message to fail, but it succeeded")
	}

	// Tampered byte test
	ciphertext1[12] ^= 0xFF
	_, err = ratchetB.DecryptMessage(ciphertext1, aad)
	if err == nil {
		t.Fatal("expected decryption of tampered ciphertext to fail, but it succeeded")
	}
}

func TestCryptoKeysZeroization(t *testing.T) {
	// 1. Test Identity.Destroy()
	ident, err := NewIdentity()
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}
	privKey := ident.PrivateKey
	ident.Destroy()

	for i, b := range privKey {
		if b != 0 {
			t.Errorf("expected identity private key byte at %d to be zero, got %d", i, b)
		}
	}

	// 2. Test RatchetState.Destroy()
	var rootKey [32]byte
	for i := range rootKey {
		rootKey[i] = byte(i + 1)
	}
	ratchet := NewRatchetState(rootKey, true)
	ratchet.Destroy()

	for i, b := range ratchet.SendChainKey {
		if b != 0 {
			t.Errorf("expected ratchet send chain key byte at %d to be zero, got %d", i, b)
		}
	}
	for i, b := range ratchet.RecvChainKey {
		if b != 0 {
			t.Errorf("expected ratchet recv chain key byte at %d to be zero, got %d", i, b)
		}
	}
}

