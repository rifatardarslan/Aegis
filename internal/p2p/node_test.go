package p2p

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/aegis-p2p/aegis/internal/transfer"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	relayclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	relayserver "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	ma "github.com/multiformats/go-multiaddr"
	"io"
	"sync"
	"testing"
	"time"
)

func testNode(t *testing.T, name string) *P2PNode {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	n, err := newNode(ctx, cancel, h, nil, name, "")
	if err != nil {
		t.Fatal(err)
	}
	n.SetDiscoverable(true)
	t.Cleanup(func() { _ = n.Close() })
	return n
}
func linkNodes(a, b *P2PNode) { a.Host.Peerstore().AddAddrs(b.Host.ID(), b.Host.Addrs(), time.Hour) }
func connectedPair(t *testing.T) (*P2PNode, *P2PNode, *Session, *Session) {
	t.Helper()
	a, b := testNode(t, "sender"), testNode(t, "receiver")
	linkNodes(a, b)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type result struct {
		s   *Session
		err error
	}
	ready := make(chan result, 1)
	go func() { s, err := a.Connect(ctx, b.Host.ID().String()); ready <- result{s, err} }()
	select {
	case event := <-b.Events:
		if event.Request == nil {
			t.Fatal("expected request")
		}
		event.Request.Decide(true)
	case <-ctx.Done():
		t.Fatal("request timeout")
	}
	var bs *Session
	select {
	case event := <-b.Events:
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		bs = event.Session
	case <-ctx.Done():
		t.Fatal("handshake timeout")
	}
	ar := <-ready
	if ar.err != nil {
		t.Fatal(ar.err)
	}
	if bs == nil {
		t.Fatal("no responder session")
	}
	return a, b, ar.s, bs
}
func TestEphemeralIdentities(t *testing.T) {
	a, b := testNode(t, "same-name"), testNode(t, "same-name")
	if a.Host.ID() == b.Host.ID() {
		t.Fatal("reused host identity")
	}
	if bytes.Equal(a.Identity.PublicKey, b.Identity.PublicKey) {
		t.Fatal("reused application identity")
	}
}
func TestNormalizeName(t *testing.T) {
	for _, name := range []string{"ab", "bad name", "bad\nname", "💥💥💥", "", "a/b"} {
		if _, err := NormalizeName(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	a, _ := NormalizeName(" ÉLISE ")
	b, _ := NormalizeName("e\u0301lise")
	if a != b {
		t.Fatal(a, b)
	}
	if NameCID(a) != NameCID(b) {
		t.Fatal("discovery mismatch")
	}
}
func TestSearchProfilesAndDuplicateNames(t *testing.T) {
	a, b, c := testNode(t, "searcher"), testNode(t, "Élise"), testNode(t, "élise")
	linkNodes(a, b)
	linkNodes(a, c)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	results, err := a.Search(ctx, "E\u0301LISE")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 matching peers, got %d", len(results))
	}
	b.SetDiscoverable(false)
	results, err = a.Search(ctx, "élise")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("hidden peer returned: %d", len(results))
	}
}
func TestRequestRejectAndBusy(t *testing.T) {
	a, b := testNode(t, "alice"), testNode(t, "bob")
	linkNodes(a, b)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := a.Connect(ctx, b.Host.ID().String()); done <- err }()
	event := <-b.Events
	if event.Request == nil {
		t.Fatal("no request")
	}
	if !b.Profile().Available && b.reserve() {
		t.Fatal("accepted second reservation")
	}
	event.Request.Decide(false)
	if err := <-done; err == nil {
		t.Fatal("accepted rejection")
	}
	<-b.Events
	// The handler releases its reservation after emitting its error.
	deadline := time.Now().Add(time.Second)
	for !b.Profile().Available && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !b.Profile().Available {
		t.Fatal("rejection left node busy")
	}
}
func TestConcurrentMessagesAndChunks(t *testing.T) {
	_, _, a, b := connectedPair(t)
	if a.PeerName != "receiver" || b.PeerName != "sender" {
		t.Fatal("peer names not propagated")
	}
	if a.SAS != b.SAS {
		t.Fatal("different transcript SAS")
	}
	const count = 40
	received := make(chan error, 1)
	go func() {
		for i := 0; i < count; i++ {
			if _, err := b.Read(); err != nil {
				received <- err
				return
			}
		}
		received <- nil
	}()
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			f := transfer.AegisFrame{Type: transfer.TypeMsg, MsgText: "hello", Index: index}
			if index%2 == 0 {
				f = transfer.AegisFrame{Type: transfer.TypeChunk, TransferID: NewID(), Index: index, Data: bytes.Repeat([]byte{1}, 1024)}
			}
			errs <- a.Send(ctx, f)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := <-received; err != nil {
		t.Fatal(err)
	}
}
func TestConnectCancelledReleasesReservation(t *testing.T) {
	a, b := testNode(t, "alice"), testNode(t, "bob")
	linkNodes(a, b)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := a.Connect(ctx, b.Host.ID().String()); done <- err }()
	event := <-b.Events
	cancel()
	if <-done == nil {
		t.Fatal("cancel accepted")
	}
	event.Request.Decide(false)
	if !a.Profile().Available {
		t.Fatal("cancel left sender busy")
	}
}
func TestIncompatiblePeer(t *testing.T) {
	a, b := testNode(t, "alice"), testNode(t, "bob")
	b.Host.RemoveStreamHandler(ProtocolID)
	linkNodes(a, b)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := a.Connect(ctx, b.Host.ID().String()); err == nil {
		t.Fatal("incompatible peer accepted")
	}
}
func TestStaleProfile(t *testing.T) {
	a, b := testNode(t, "alice"), testNode(t, "bob")
	linkNodes(a, b)
	info := peer.AddrInfo{ID: b.Host.ID(), Addrs: b.Host.Addrs()}
	_ = b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := a.QueryProfile(ctx, info); err == nil {
		t.Fatal("offline profile accepted")
	}
}

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(b []byte) (int, error) {
	if len(b) > 2 {
		b = b[:2]
	}
	return w.Buffer.Write(b)
}
func TestStreamFraming(t *testing.T) {
	var w shortWriter
	payload := []byte("hello-aegis")
	if err := WriteFrame(&w, payload); err != nil {
		t.Fatal(err)
	}
	read, err := ReadFrame(&w)
	if err != nil || !bytes.Equal(payload, read) {
		t.Fatal(err, read)
	}
}
func TestFrameLimits(t *testing.T) {
	for _, size := range []uint32{0, 512*1024 + 1} {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.BigEndian, size)
		if _, err := ReadFrame(&b); err == nil {
			t.Fatal("accepted invalid size")
		}
	}
	if err := WriteFrame(io.Discard, make([]byte, 512*1024+1)); err == nil {
		t.Fatal("oversize send accepted")
	}
}

func TestCustomCircuitRelay(t *testing.T) {
	relay, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	service, err := relayserver.New(relay, relayserver.WithReservationAddressFilter(func(ma.Multiaddr) bool { return true }))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	a, b := testNode(t, "alice"), testNode(t, "bob")
	relayInfo := peer.AddrInfo{ID: relay.ID(), Addrs: relay.Addrs()}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := b.Host.Connect(ctx, relayInfo); err != nil {
		t.Fatal(err)
	}
	if _, err := relayclient.Reserve(ctx, b.Host, relayInfo); err != nil {
		t.Fatal(err)
	}
	relayAddress := relay.Addrs()[0].String() + "/p2p/" + relay.ID().String()
	a.customRelay = relayAddress
	target := relayAddress + "/p2p-circuit/p2p/" + b.Host.ID().String()
	type result struct {
		s   *Session
		err error
	}
	done := make(chan result, 1)
	go func() { s, err := a.Connect(ctx, target); done <- result{s, err} }()
	select {
	case event := <-b.Events:
		if event.Request == nil {
			t.Fatal("no relay request")
		}
		event.Request.Decide(true)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var responder *Session
	select {
	case event := <-b.Events:
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		responder = event.Session
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	outgoing := <-done
	if outgoing.err != nil {
		t.Fatal(outgoing.err)
	}
	if outgoing.s.ConnectionType() != "Relay" {
		t.Fatal("test did not use relay")
	}
	read := make(chan error, 1)
	go func() {
		frame, err := responder.Read()
		if err == nil && frame.MsgText != "via relay" {
			err = io.ErrUnexpectedEOF
		}
		read <- err
	}()
	if err := outgoing.s.Send(ctx, transfer.AegisFrame{Type: transfer.TypeMsg, MsgText: "via relay"}); err != nil {
		t.Fatal(err)
	}
	if err := <-read; err != nil {
		t.Fatal(err)
	}
}

func TestNameDiscoveryThroughDHT(t *testing.T) {
	router := testNode(t, "router")
	a, b := testNode(t, "searcher"), testNode(t, "discoverable")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, n := range []*P2PNode{router, a, b} {
		kd, err := dht.New(n.ctx, n.Host, dht.Mode(dht.ModeServer), dht.DisableAutoRefresh())
		if err != nil {
			t.Fatal(err)
		}
		n.Discovery = &DiscoveryManager{DHT: kd, host: n.Host}
	}
	info := peer.AddrInfo{ID: router.Host.ID(), Addrs: router.Host.Addrs()}
	for _, n := range []*P2PNode{a, b} {
		if err := n.Host.Connect(ctx, info); err != nil {
			t.Fatal(err)
		}
		if !n.Discovery.waitForBootstrap(ctx, 3*time.Second) {
			t.Fatal("local DHT did not become ready")
		}
	}
	if err := b.Discovery.DHT.Provide(ctx, NameCID("discoverable"), true); err != nil {
		t.Fatal(err)
	}
	// The searcher initially knows only the router; lookup must locate the provider.
	results, err := a.Search(ctx, "DISCOVERABLE")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, result := range results {
		if result.Profile.PeerID == b.Host.ID().String() {
			found = true
		}
	}
	if !found {
		t.Fatal("name provider not discovered through DHT")
	}
}

func TestSessionCloseUnblocksPendingSends(t *testing.T) {
	_, _, a, _ := connectedPair(t)
	var wg sync.WaitGroup
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = a.Send(ctx, transfer.AegisFrame{Type: transfer.TypeMsg, MsgText: "pending"})
		}()
	}
	a.Close()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("close left sends blocked")
	}
	if err := a.Send(context.Background(), transfer.AegisFrame{Type: transfer.TypeMsg, MsgText: "closed"}); err == nil {
		t.Fatal("closed session accepted send")
	}
}
