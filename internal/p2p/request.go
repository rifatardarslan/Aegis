package p2p

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aegis-p2p/aegis/internal/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"net"
	"strings"
	"sync"
	"time"
)

const RequestTimeout = 60 * time.Second

type requestWire struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
}
type Event struct {
	Request *IncomingRequest
	Session *Session
	Err     error
}
type IncomingRequest struct {
	ID       string
	Name     string
	PeerID   string
	Expires  time.Time
	decision chan bool
	once     sync.Once
}

func (r *IncomingRequest) Decide(accept bool) { r.once.Do(func() { r.decision <- accept }) }

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func controlWrite(s network.Stream, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return WriteFrame(s, b)
}
func controlRead(s network.Stream, v any) error {
	b, err := readControl(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (n *P2PNode) reserve() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed || n.busy {
		return false
	}
	n.busy = true
	return true
}
func (n *P2PNode) release() { n.mu.Lock(); n.busy = false; n.pending = nil; n.mu.Unlock() }
func (n *P2PNode) setPending(s network.Stream) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		_ = s.Reset()
		return false
	}
	n.pending = s
	return true
}
func (n *P2PNode) emit(e Event) {
	select {
	case n.Events <- e:
	case <-n.ctx.Done():
	}
}

func (n *P2PNode) handshake(s network.Stream, id string, initiator bool) (*Session, error) {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil, context.Canceled
	}
	ident := &crypto.Identity{PublicKey: append([]byte(nil), n.Identity.PublicKey...), PrivateKey: append([]byte(nil), n.Identity.PrivateKey...)}
	n.mu.Unlock()
	local, remote := s.Conn().LocalPeer().String(), s.Conn().RemotePeer().String()
	if !initiator {
		local, remote = remote, local
	}
	binding, _ := json.Marshal([]string{ProtocolID, id, local, remote})
	_ = s.SetDeadline(time.Now().Add(15 * time.Second))
	result, err := crypto.RunBoundHandshake(s, ident, initiator, binding)
	if err != nil {
		ident.Destroy()
		return nil, err
	}
	session := newSession(s, id, result, initiator)
	crypto.Array32(&result.SharedRootKey)
	_ = s.SetDeadline(time.Time{})
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		session.Close()
		return nil, context.Canceled
	}
	n.active = session
	n.pending = nil
	n.mu.Unlock()
	return session, nil
}

// Connect sends a request and waits for explicit acceptance, then establishes v2.
func (n *P2PNode) Connect(ctx context.Context, target string) (*Session, error) {
	if !n.reserve() {
		return nil, fmt.Errorf("already waiting for a request or in a session")
	}
	success := false
	defer func() {
		if !success {
			n.release()
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()
	stopNode := context.AfterFunc(n.ctx, cancel)
	defer stopNode()
	s, err := n.dial(ctx, target)
	if err != nil {
		return nil, err
	}
	if !n.setPending(s) {
		return nil, context.Canceled
	}
	stop := context.AfterFunc(ctx, func() { _ = s.Reset() })
	defer stop()
	defer func() {
		if !success {
			_ = s.Reset()
		}
	}()
	_ = s.SetDeadline(time.Now().Add(RequestTimeout))
	req := requestWire{ID: NewID(), Name: n.Profile().Name}
	if err := controlWrite(s, req); err != nil {
		return nil, err
	}
	var reply requestWire
	if err := controlRead(s, &reply); err != nil {
		return nil, fmt.Errorf("request ended or timed out: %w", err)
	}
	if reply.ID != req.ID {
		return nil, fmt.Errorf("invalid response request ID")
	}
	if reply.Status != "accept" && reply.Status != "busy" && reply.Status != "rejected" {
		return nil, fmt.Errorf("invalid request response")
	}
	if reply.Status != "accept" {
		return nil, fmt.Errorf("connection request: %s", reply.Status)
	}
	if _, err := NormalizeName(reply.Name); err != nil {
		return nil, fmt.Errorf("invalid responder name")
	}
	session, err := n.handshake(s, req.ID, true)
	if err != nil {
		return nil, err
	}
	session.PeerName = strings.TrimSpace(reply.Name)
	success = true
	return session, nil
}

func (n *P2PNode) handleRequest(s network.Stream) {
	if !n.requestLimiter.Allow() {
		_ = s.Reset()
		return
	}
	_ = s.SetDeadline(time.Now().Add(5 * time.Second))
	var req requestWire
	if err := controlRead(s, &req); err != nil {
		_ = s.Reset()
		return
	}
	if b, err := hex.DecodeString(req.ID); err != nil || len(b) != 16 {
		_ = s.Reset()
		return
	}
	if _, err := NormalizeName(req.Name); err != nil || req.Status != "" {
		_ = s.Reset()
		return
	}
	if !n.reserve() {
		_ = controlWrite(s, requestWire{ID: req.ID, Status: "busy"})
		_ = s.Close()
		return
	}
	success := false
	defer func() {
		if !success {
			_ = s.Reset()
			n.release()
		}
	}()
	if !n.setPending(s) {
		return
	}
	expires := time.Now().Add(RequestTimeout)
	_ = s.SetDeadline(expires)
	r := &IncomingRequest{ID: req.ID, Name: strings.TrimSpace(req.Name), PeerID: s.Conn().RemotePeer().String(), Expires: expires, decision: make(chan bool, 1)}
	n.emit(Event{Request: r})
	peerGone := make(chan error, 1)
	go func() {
		var b [1]byte
		_, err := s.Read(b[:])
		if err == nil {
			err = fmt.Errorf("unexpected data before acceptance")
		}
		peerGone <- err
	}()
	timer := time.NewTimer(time.Until(expires))
	defer timer.Stop()
	select {
	case <-n.ctx.Done():
		return
	case <-timer.C:
		n.emit(Event{Err: fmt.Errorf("connection request timed out")})
		return
	case <-peerGone:
		n.emit(Event{Err: fmt.Errorf("connection request cancelled by peer")})
		return
	case accept := <-r.decision:
		_ = s.SetReadDeadline(time.Now())
		readErr := <-peerGone
		var timeout net.Error
		if !errors.As(readErr, &timeout) || !timeout.Timeout() {
			n.emit(Event{Err: fmt.Errorf("connection request cancelled by peer")})
			return
		}
		_ = s.SetDeadline(expires)
		if !accept {
			_ = controlWrite(s, requestWire{ID: req.ID, Status: "rejected"})
			n.emit(Event{Err: fmt.Errorf("connection request rejected")})
			return
		}
	}
	if err := controlWrite(s, requestWire{ID: req.ID, Name: n.Profile().Name, Status: "accept"}); err != nil {
		n.emit(Event{Err: err})
		return
	}
	session, err := n.handshake(s, req.ID, false)
	if err != nil {
		n.emit(Event{Err: err})
		return
	}
	session.PeerName = strings.TrimSpace(req.Name)
	success = true
	n.emit(Event{Session: session})
}
