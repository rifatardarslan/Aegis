package p2p

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/aegis-p2p/aegis/internal/crypto"
)

// RateLimiter implements a thread-safe token bucket algorithm for secure rate limiting.
type RateLimiter struct {
	mu           sync.Mutex
	tokens       float64
	maxTokens    float64
	refillRate   float64 // tokens per second
	lastRefilled time.Time
}

// NewRateLimiter constructs a new RateLimiter.
func NewRateLimiter(refillRate float64, maxTokens float64) *RateLimiter {
	return &RateLimiter{
		tokens:       maxTokens,
		maxTokens:    maxTokens,
		refillRate:   refillRate,
		lastRefilled: time.Now(),
	}
}

// Allow checks if a request is allowed according to the token bucket rate limiter.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefilled).Seconds()
	rl.lastRefilled = now

	rl.tokens += elapsed * rl.refillRate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}

	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}
	return false
}

// P2PNode coordinates libp2p host, discovery, active stream, identities, and ratchets.
type P2PNode struct {
	Host             host.Host
	Discovery        *DiscoveryManager
	ActiveStream     network.Stream
	Identity         *crypto.Identity
	Ratchet          *crypto.RatchetState
	RemotePubKey     ed25519.PublicKey
	ctx              context.Context
	cancel           context.CancelFunc
	customRelay      string
	IncomingStreamCh chan network.Stream
	Limiter          *RateLimiter
}

// NewP2PNode initializes a new deterministic libp2p node and generates a fresh ephemeral identity.
func NewP2PNode(passphrase string, customRelay string) (*P2PNode, error) {
	ctx, cancel := context.WithCancel(context.Background())

	h, err := NewHost(ctx, passphrase, customRelay)
	if err != nil {
		cancel()
		return nil, err
	}

	dm, err := NewDiscoveryManager(ctx, h, customRelay)
	if err != nil {
		h.Close()
		cancel()
		return nil, err
	}

	ident, err := crypto.NewIdentity()
	if err != nil {
		dm.Close()
		h.Close()
		cancel()
		return nil, err
	}

	node := &P2PNode{
		Host:             h,
		Discovery:        dm,
		Identity:         ident,
		ctx:              ctx,
		cancel:           cancel,
		customRelay:      customRelay,
		IncomingStreamCh: make(chan network.Stream, 1),
		Limiter:          NewRateLimiter(150, 200),
	}

	// Register protocol handler for /aegis/1.0.0
	h.SetStreamHandler(ProtocolID, func(s network.Stream) {
		select {
		case node.IncomingStreamCh <- s:
			// Stream successfully passed to TUI dispatcher
		default:
			// If already in a session or busy, reject incoming stream.
			_ = s.Reset()
		}
	})

	// Start rendezvous-based DHT advertising so this node is discoverable
	// by other Aegis peers on different networks via FindProviders().
	dm.StartAdvertising(ctx)

	return node, nil
}

// Connect attempts to find a peer, connect, open a stream, and run the initiator handshake.
// It supports both a raw PeerID string and a full Multiaddress string containing a PeerID.
func (n *P2PNode) Connect(ctx context.Context, peerIDOrMultiaddr string) (network.Stream, *crypto.HandshakeResult, error) {
	var targetID peer.ID
	var addrInfo peer.AddrInfo
	var err error

	// Try to parse as Multiaddress first
	if ma, errAddr := multiaddr.NewMultiaddr(peerIDOrMultiaddr); errAddr == nil {
		info, errInfo := peer.AddrInfoFromP2pAddr(ma)
		if errInfo == nil {
			targetID = info.ID
			addrInfo = *info
			// Add the address to Peerstore so libp2p knows how to reach it
			n.Host.Peerstore().AddAddrs(targetID, addrInfo.Addrs, time.Hour)
		} else {
			return nil, nil, fmt.Errorf("invalid multiaddress format: %w", errInfo)
		}
	} else {
		// Fallback to decoding as raw PeerID
		targetID, err = peer.Decode(peerIDOrMultiaddr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid PeerID or Multiaddr format: %w", err)
		}

		// Find Peer multiaddrs via DHT/Peerstore
		addrInfo, err = n.Discovery.FindPeer(ctx, targetID)
		if err != nil {
			return nil, nil, err
		}
	}

	// 2. Connect to the peer
	err = n.Host.Connect(ctx, addrInfo)
	if err != nil {
		// Connection failed. It could be due to outdated cached addresses in Peerstore.
		// Clear peerstore cache for this target and attempt a fresh DHT lookup.
		n.Host.Peerstore().ClearAddrs(targetID)

		freshAddrInfo, freshErr := n.Discovery.FindPeer(ctx, targetID)
		if freshErr == nil && len(freshAddrInfo.Addrs) > 0 {
			addrInfo = freshAddrInfo
			err = n.Host.Connect(ctx, addrInfo)
		}
	}

	// 3. If direct connection (cached or fresh) still failed, attempt relay fallback
	if err != nil {
		relayAddrs := n.buildRelayCircuitAddrs(targetID)
		if len(relayAddrs) == 0 {
			return nil, nil, fmt.Errorf("failed to connect to peer: %w\n  hint: relay not ready — wait for ADDRESS field to show a /p2p-circuit address", err)
		}
		relayAddrInfo := peer.AddrInfo{
			ID:    targetID,
			Addrs: relayAddrs,
		}
		n.Host.Peerstore().AddAddrs(targetID, relayAddrs, time.Hour)
		if err2 := n.Host.Connect(ctx, relayAddrInfo); err2 != nil {
			return nil, nil, fmt.Errorf("failed to connect to peer (direct and relay): %w", err2)
		}
	}

	// 3. Open stream
	stream, err := n.Host.NewStream(ctx, targetID, ProtocolID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open protocol stream: %w", err)
	}

	// 4. Run initiator handshake (X25519 exchange + Ed25519 auth) with a 15-second deadline
	_ = stream.SetDeadline(time.Now().Add(15 * time.Second))
	res, err := crypto.RunHandshake(stream, n.Identity, true)
	if err != nil {
		_ = stream.Reset()
		return nil, nil, fmt.Errorf("cryptographic handshake failed: %w", err)
	}
	_ = stream.SetDeadline(time.Time{}) // Reset stream deadlines for normal operations

	n.Ratchet = crypto.NewRatchetState(res.SharedRootKey, true)
	n.RemotePubKey = res.RemotePubKey
	n.ActiveStream = stream
	crypto.Array32(&res.SharedRootKey) // Zeroize root key copy from RAM
	return stream, res, nil
}

// buildRelayCircuitAddrs constructs /p2p-circuit multiaddrs for all known relay nodes,
// targeting the given peer. Pre-existing connection is NOT required — go-libp2p's
// circuit relay v2 dialer will connect to the relay automatically during the dial.
func (n *P2PNode) buildRelayCircuitAddrs(targetID peer.ID) []multiaddr.Multiaddr {
	targetPart, err := multiaddr.NewMultiaddr("/p2p/" + targetID.String())
	if err != nil {
		return nil
	}
	circuitPart, err := multiaddr.NewMultiaddr("/p2p-circuit")
	if err != nil {
		return nil
	}

	var addrs []multiaddr.Multiaddr
	seen := make(map[string]bool)

	for _, relayInfo := range GetRelayAddrInfos() {
		relayPart, err := multiaddr.NewMultiaddr("/p2p/" + relayInfo.ID.String())
		if err != nil {
			continue
		}
		// Prefer peerstore addresses (may include resolved DNS/IP addresses).
		// Fall back to the original parsed addresses if peerstore has nothing.
		relayAddrs := n.Host.Peerstore().Addrs(relayInfo.ID)
		if len(relayAddrs) == 0 {
			relayAddrs = relayInfo.Addrs
		}
		for _, relayAddr := range relayAddrs {
			circuit := relayAddr.
				Encapsulate(relayPart).
				Encapsulate(circuitPart).
				Encapsulate(targetPart)
			key := circuit.String()
			if !seen[key] {
				seen[key] = true
				addrs = append(addrs, circuit)
			}
		}
	}
	return addrs
}

// HasRelayAddress reports whether the local node has an active /p2p-circuit relay address.
func (n *P2PNode) HasRelayAddress() bool {
	for _, addr := range n.Host.Addrs() {
		if strings.Contains(addr.String(), "/p2p-circuit") {
			return true
		}
	}
	return false
}

// GetConnectionType checks if the active stream is relayed or direct.
func (n *P2PNode) GetConnectionType() string {
	if n.ActiveStream == nil {
		return "Unknown"
	}
	conn := n.ActiveStream.Conn()
	if conn == nil {
		return "Unknown"
	}
	remoteAddr := conn.RemoteMultiaddr().String()
	if strings.Contains(remoteAddr, "/p2p-circuit") {
		return "Relay"
	}
	return "Direct"
}

// isUsableIP returns true if the IP string is a routable LAN or public address.
// Rejects loopback (127.x, ::1), any-address (0.0.0.0), and link-local (169.254.x, fe80::).
func isUsableIP(addrStr string) bool {
	bad := []string{"/127.0.0.1", "/::1", "/0.0.0.0", "/169.254.", "/fe80::"}
	for _, b := range bad {
		if strings.Contains(addrStr, b) {
			return false
		}
	}
	return true
}

// GetBestAddress returns the most connectable multiaddress string for this node.
// Priority: relay (p2p-circuit) > routable LAN/public IP > link-local > peerID.
func (n *P2PNode) GetBestAddress() string {
	addrs := n.Host.Addrs()
	peerID := n.Host.ID().String()

	// 1. Relay address — works across internet
	for _, addr := range addrs {
		if strings.Contains(addr.String(), "/p2p-circuit") {
			return fmt.Sprintf("%s/p2p/%s", addr.String(), peerID)
		}
	}

	// 2. Routable LAN or public IP (skip loopback, any-addr, link-local)
	for _, addr := range addrs {
		if isUsableIP(addr.String()) {
			return fmt.Sprintf("%s/p2p/%s", addr.String(), peerID)
		}
	}

	// 3. Fallback: scan interfaces for a routable IPv4 using the TCP port libp2p chose
	var tcpPort string
	for _, addr := range addrs {
		addrStr := addr.String()
		if strings.Contains(addrStr, "/tcp/") {
			parts := strings.Split(addrStr, "/tcp/")
			if len(parts) > 1 {
				tcpPort = strings.Split(parts[1], "/")[0]
				break
			}
		}
	}
	if tcpPort != "" {
		ifaces, err := net.Interfaces()
		if err == nil {
			for _, iface := range ifaces {
				if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
					continue
				}
				addrsIface, _ := iface.Addrs()
				for _, a := range addrsIface {
					var ip net.IP
					switch v := a.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					}
					if ip == nil || ip.To4() == nil {
						continue
					}
					ipStr := ip.String()
					candidate := "/ip4/" + ipStr
					if isUsableIP(candidate) {
						return fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", ipStr, tcpPort, peerID)
					}
				}
			}
		}
	}

	// 4. Last resort: first address libp2p knows about
	if len(addrs) > 0 {
		return fmt.Sprintf("%s/p2p/%s", addrs[0].String(), peerID)
	}
	return peerID
}

// GetAddresses returns all full multiaddresses for this node as a list of strings.
func (n *P2PNode) GetAddresses() []string {
	var addrs []string
	peerID := n.Host.ID().String()
	for _, addr := range n.Host.Addrs() {
		addrs = append(addrs, fmt.Sprintf("%s/p2p/%s", addr.String(), peerID))
	}
	return addrs
}

// Close gracefully terminates the node and zeroizes all active key materials.
func (n *P2PNode) Close() error {
	n.cancel()
	if n.ActiveStream != nil {
		_ = n.ActiveStream.Close()
	}
	if n.Ratchet != nil {
		n.Ratchet.Destroy()
	}
	if n.Identity != nil {
		n.Identity.Destroy()
	}
	_ = n.Discovery.Close()
	_ = n.Host.Close()
	return nil
}
