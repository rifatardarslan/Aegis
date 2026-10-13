package p2p

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"github.com/aegis-p2p/aegis/internal/crypto"
	"github.com/aegis-p2p/aegis/internal/transfer"
	"github.com/libp2p/go-libp2p/core/network"
	"strings"
	"sync"
	"time"
)

type sendJob struct {
	frame  transfer.AegisFrame
	result chan error
}

// Session owns all cipher state. A single worker serializes encryption and writes.
type Session struct {
	PeerName               string
	ID                     string
	LocalFP, RemoteFP, SAS string
	stream                 network.Stream
	ratchet                *crypto.RatchetState
	identity               *crypto.Identity
	remote                 ed25519.PublicKey
	aad                    []byte
	queue                  chan sendJob
	done                   chan struct{}
	workerDone             chan struct{}
	queueMu                sync.Mutex
	readMu                 sync.Mutex
	closeOnce              sync.Once
	limiter                *RateLimiter
}

func newSession(stream network.Stream, id string, result *crypto.HandshakeResult, initiator bool) *Session {
	s := &Session{ID: id, stream: stream, ratchet: crypto.NewRatchetState(result.SharedRootKey, initiator), identity: result.LocalIdentity, remote: result.RemotePubKey, LocalFP: crypto.Fingerprint(result.LocalIdentity.PublicKey), RemoteFP: crypto.Fingerprint(result.RemotePubKey), SAS: crypto.TranscriptSAS(result.TranscriptHash), queue: make(chan sendJob, 32), done: make(chan struct{}), workerDone: make(chan struct{}), limiter: NewRateLimiter(1000, 2000)}
	s.aad = append([]byte(nil), result.TranscriptHash[:]...)
	go s.writer()
	return s
}
func (s *Session) Done() <-chan struct{} { return s.done }
func (s *Session) PeerID() string        { return s.stream.Conn().RemotePeer().String() }
func (s *Session) ConnectionType() string {
	if strings.Contains(s.stream.Conn().RemoteMultiaddr().String(), "/p2p-circuit") {
		return "Relay"
	}
	return "Direct"
}

func (s *Session) Send(ctx context.Context, frame transfer.AegisFrame) error {
	frame.Data = append([]byte(nil), frame.Data...)
	job := sendJob{frame: frame, result: make(chan error, 1)}
	s.queueMu.Lock()
	select {
	case <-s.done:
		s.queueMu.Unlock()
		crypto.Bytes(frame.Data)
		return fmt.Errorf("session closed")
	case <-ctx.Done():
		s.queueMu.Unlock()
		crypto.Bytes(frame.Data)
		return ctx.Err()
	default:
	}
	select {
	case s.queue <- job:
		s.queueMu.Unlock()
	case <-s.done:
		s.queueMu.Unlock()
		crypto.Bytes(frame.Data)
		return fmt.Errorf("session closed")
	case <-ctx.Done():
		s.queueMu.Unlock()
		crypto.Bytes(frame.Data)
		return ctx.Err()
	}

	select {
	case err := <-job.result:
		return err
	case <-s.done:
		return fmt.Errorf("session closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) writer() {
	defer close(s.workerDone)
	defer func() {
		for {
			select {
			case job := <-s.queue:
				crypto.Bytes(job.frame.Data)
				job.result <- fmt.Errorf("session closed")
			default:
				return
			}
		}
	}()
	for {
		select {
		case <-s.done:
			return
		case job := <-s.queue:
			err := s.write(job.frame)
			crypto.Bytes(job.frame.Data)
			job.result <- err
			if err != nil {
				s.stop()
				return
			}
		}
	}
}
func (s *Session) write(frame transfer.AegisFrame) error {
	if err := frame.Validate(); err != nil {
		return err
	}
	plaintext, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	defer crypto.Bytes(plaintext)
	ciphertext, err := s.ratchet.EncryptMessage(plaintext, s.aad)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(s.identity.PrivateKey, ciphertext)
	packet := append(sig, ciphertext...)
	_ = s.stream.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return WriteFrame(s.stream, packet)
}
func (s *Session) Read() (transfer.AegisFrame, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	var frame transfer.AegisFrame
	packet, err := ReadFrame(s.stream)
	if err != nil {
		return frame, err
	}
	if !s.limiter.Allow() {
		return frame, fmt.Errorf("packet rate exceeded")
	}
	if len(packet) < 64 || !ed25519.Verify(s.remote, packet[64:], packet[:64]) {
		return frame, fmt.Errorf("invalid packet signature")
	}
	plaintext, err := s.ratchet.DecryptMessage(packet[64:], s.aad)
	if err != nil {
		return frame, err
	}
	defer crypto.Bytes(plaintext)
	if err := json.Unmarshal(plaintext, &frame); err != nil {
		return frame, err
	}
	if err := frame.Validate(); err != nil {
		crypto.Bytes(frame.Data)
		return frame, err
	}
	return frame, nil
}
func (s *Session) stop() {
	s.closeOnce.Do(func() { close(s.done); s.queueMu.Lock(); s.queueMu.Unlock(); _ = s.stream.Reset() })
}
func (s *Session) Close() {
	s.stop()
	<-s.workerDone
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.ratchet.Destroy()
	s.identity.Destroy()
}
