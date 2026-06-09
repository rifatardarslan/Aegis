package p2p

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"golang.org/x/crypto/argon2"

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

	aegiscrypto "github.com/aegis-p2p/aegis/internal/crypto"
)

// DeriveHostKey generates a deterministic Ed25519 private key from the given
// passphrase using Argon2id.
func DeriveHostKey(passphrase string) (crypto.PrivKey, error) {
	// NOTE: Passphrase zeroization is handled by the caller (initP2PCmd) at the boundary layer.
	// Do NOT zero passphrase here — Go strings share backing arrays, so zeroizing the local
	// copy would corrupt the caller's copy if they need to use it again.

	// Argon2id parameters:
	// - time: 3 iterations
	// - memory: 64 MB (64 * 1024 KB)
	// - threads (parallelism): 4
	// - keyLen: 32 bytes (for Ed25519 seed)
	// Salt is static and memory-only.
	salt := []byte("aegis-deterministic-libp2p-salt-v1")
	passBytes := []byte(passphrase)
	defer aegiscrypto.Bytes(passBytes)
	seed := argon2.IDKey(passBytes, salt, 3, 64*1024, 4, 32)
	defer aegiscrypto.Bytes(seed)

	stdPriv := ed25519.NewKeyFromSeed(seed)
	return crypto.UnmarshalEd25519PrivateKey(stdPriv)
}

// NewHost creates a new libp2p host deterministically derived from a passphrase.
func NewHost(ctx context.Context, passphrase string, customRelayAddr string) (host.Host, error) {
	privKey, err := DeriveHostKey(passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to derive host key: %w", err)
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
			autorelay.WithNumRelays(1),       // only 1 relay needed — default was 4 (= all static nodes)
			autorelay.WithMinCandidates(1),   // start trying immediately with 1 candidate
			autorelay.WithBootDelay(0),       // no initial wait — default was 3 minutes
			autorelay.WithBackoff(5*time.Minute),      // retry each failed node after 5m, not 1 hour/30s
			autorelay.WithMinInterval(5*time.Minute),  // re-query peer source every 5m
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
