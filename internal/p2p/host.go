package p2p

import (
	"context"
	"crypto/rand"
	"fmt"

	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	libp2pws "github.com/libp2p/go-libp2p/p2p/transport/websocket"
	"github.com/multiformats/go-multiaddr"
)

// NewHost creates an ephemeral libp2p identity. Names never derive private keys.
func NewHost(ctx context.Context, customRelayAddr string) (host.Host, error) {
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate host identity: %w", err)
	}
	if customRelayAddr != "" {
		ma, err := multiaddr.NewMultiaddr(customRelayAddr)
		if err != nil {
			return nil, fmt.Errorf("invalid relay: %w", err)
		}
		if _, err := peer.AddrInfoFromP2pAddr(ma); err != nil {
			return nil, fmt.Errorf("invalid relay: %w", err)
		}
	}

	relays := GetRelayAddrInfos()
	if customRelayAddr != "" {
		// Parse and prepend custom relay if specified
		if ma, err := multiaddr.NewMultiaddr(customRelayAddr); err == nil {
			if info, err := peer.AddrInfoFromP2pAddr(ma); err == nil {
				relays = append([]peer.AddrInfo{*info}, relays...)
			}
		}
	}

	h, err := libp2p.New(
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/0",
			"/ip4/0.0.0.0/udp/0/quic-v1",
			"/ip4/0.0.0.0/tcp/0/ws", // WebSocket — allows connecting to relays on port 443/WSS
		),
		libp2p.Identity(privKey),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(libp2pws.New),
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),
		libp2p.EnableAutoRelayWithStaticRelays(relays,
			autorelay.WithNumRelays(1),               // only 1 relay needed — default was 4 (= all static nodes)
			autorelay.WithMinCandidates(1),           // start trying immediately with 1 candidate
			autorelay.WithBootDelay(0),               // no initial wait — default was 3 minutes
			autorelay.WithBackoff(5*time.Minute),     // retry each failed node after 5m, not 1 hour/30s
			autorelay.WithMinInterval(5*time.Minute), // re-query peer source every 5m
		),
		libp2p.EnableNATService(),
		// Force private-reachability mode so auto-relay always establishes
		// relay reservations. Without this, AutoNAT may report the node as
		// "public" (CGNAT external IP looks routable) and skip relay setup.
		libp2p.ForceReachabilityPrivate(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize libp2p host: %w", err)
	}

	return h, nil
}
