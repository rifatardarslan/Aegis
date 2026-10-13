package p2p

import (
	"context"
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

// P2PNode owns one ephemeral identity and at most one pending/active session.
type P2PNode struct {
	Host            host.Host
	Discovery       *DiscoveryManager
	Identity        *crypto.Identity
	Events          chan Event
	ctx             context.Context
	cancel          context.CancelFunc
	customRelay     string
	mu              sync.Mutex
	name            string
	discoverable    bool
	busy            bool
	closed          bool
	active          *Session
	pending         network.Stream
	advertiseCancel context.CancelFunc
	profileLimiter  *RateLimiter
	requestLimiter  *RateLimiter
	closeOnce       sync.Once
}

func NewP2PNode(name, customRelay string) (*P2PNode, error) {
	if _, err := NormalizeName(name); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	h, err := NewHost(ctx, customRelay)
	if err != nil {
		cancel()
		return nil, err
	}
	dm, err := NewDiscoveryManager(ctx, h, customRelay)
	if err != nil {
		_ = h.Close()
		cancel()
		return nil, err
	}
	n, err := newNode(ctx, cancel, h, dm, name, customRelay)
	if err != nil {
		_ = dm.Close()
		_ = h.Close()
		cancel()
		return nil, err
	}
	n.SetDiscoverable(true)
	return n, nil
}

func newNode(ctx context.Context, cancel context.CancelFunc, h host.Host, dm *DiscoveryManager, name, relay string) (*P2PNode, error) {
	ident, err := crypto.NewIdentity()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	n := &P2PNode{Host: h, Discovery: dm, Identity: ident, ctx: ctx, cancel: cancel, name: name, customRelay: relay, Events: make(chan Event, 8), profileLimiter: NewRateLimiter(5, 10), requestLimiter: NewRateLimiter(1, 5)}
	h.SetStreamHandler(ProfileProtocol, n.handleProfile)
	h.SetStreamHandler(ProtocolID, n.handleRequest)
	return n, nil
}

func (n *P2PNode) Done() <-chan struct{} { return n.ctx.Done() }

func (n *P2PNode) dial(ctx context.Context, target string) (network.Stream, error) {
	var pi peer.AddrInfo
	if ma, err := multiaddr.NewMultiaddr(target); err == nil {
		info, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			return nil, err
		}
		pi = *info
	} else {
		id, err := peer.Decode(target)
		if err != nil {
			return nil, fmt.Errorf("invalid Peer ID or address")
		}
		if id == n.Host.ID() {
			return nil, fmt.Errorf("cannot connect to yourself")
		}
		pi = peer.AddrInfo{ID: id, Addrs: n.Host.Peerstore().Addrs(id)}
		if len(pi.Addrs) == 0 && n.Discovery != nil {
			pi, err = n.Discovery.FindPeer(ctx, id)
			if err != nil {
				return nil, err
			}
		}
	}
	if pi.ID == n.Host.ID() {
		return nil, fmt.Errorf("cannot connect to yourself")
	}
	if err := n.Host.Connect(ctx, pi); err != nil {
		n.Host.Peerstore().ClearAddrs(pi.ID)
		if n.Discovery != nil {
			if fresh, e := n.Discovery.FindPeer(ctx, pi.ID); e == nil {
				pi = fresh
			}
		}
		if e := n.Host.Connect(ctx, pi); e != nil {
			pi.Addrs = n.buildRelayCircuitAddrs(pi.ID)
			if e = n.Host.Connect(ctx, pi); e != nil {
				return nil, fmt.Errorf("peer unreachable (direct and relay): %w", e)
			}
		}
	}
	s, err := n.Host.NewStream(network.WithAllowLimitedConn(ctx, "aegis session"), pi.ID, ProtocolID)
	if err != nil {
		return nil, fmt.Errorf("peer does not support Aegis v2: %w", err)
	}
	return s, nil
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

	for _, relayInfo := range relayAddrInfos(n.customRelay) {
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

// Close cancels pending operations before destroying owned secrets.
func (n *P2PNode) Close() error {
	n.closeOnce.Do(func() {
		n.mu.Lock()
		n.closed = true
		n.cancel()
		pending := n.pending
		active := n.active
		n.mu.Unlock()
		if pending != nil {
			_ = pending.Reset()
		}
		if active != nil {
			active.Close()
		}
		if n.Discovery != nil {
			_ = n.Discovery.Close()
		}
		_ = n.Host.Close()
		// Handshakes use their own identity copy; closing cannot race with signing.
		n.mu.Lock()
		n.Identity.Destroy()
		n.mu.Unlock()
	})
	return nil
}
