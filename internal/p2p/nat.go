package p2p

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// PublicRelays lists public libp2p circuit relay v2 nodes.
// Verified 2026-06: ny5 (51.81.93.51) accepts reservations; others connect but refuse.
// All IPs are direct to avoid DNS resolution failures on mobile networks.
var PublicRelays = []string{
	// ny5.bootstrap.libp2p.io (51.81.93.51) — CONFIRMED accepts relay reservations
	"/ip4/51.81.93.51/tcp/4001/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/ip4/51.81.93.51/udp/4001/quic-v1/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	// ny5 WSS on port 443 — works on mobile carriers that block port 4001
	"/dns/ny5.bootstrap.libp2p.io/tcp/443/wss/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	// sv15.bootstrap.libp2p.io (147.135.44.132)
	"/ip4/147.135.44.132/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	"/ip4/147.135.44.132/udp/4001/quic-v1/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	// am6.bootstrap.libp2p.io (54.38.47.166)
	"/ip4/54.38.47.166/tcp/4001/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	"/ip4/54.38.47.166/udp/4001/quic-v1/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	// sg1.bootstrap.libp2p.io (15.235.144.210)
	"/ip4/15.235.144.210/tcp/4001/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
	"/ip4/15.235.144.210/udp/4001/quic-v1/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
	// dnsaddr fallbacks (lazy DNS, includes all transports the node advertises)
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
}

// GetRelayAddrInfos parses the public relays into peer.AddrInfo structures,
// merging all addresses for the same peer ID into a single AddrInfo.
// This ensures auto-relay and circuit dial can try all transports (TCP, QUIC) for each relay.
func GetRelayAddrInfos() []peer.AddrInfo {
	merged := make(map[peer.ID]*peer.AddrInfo)
	var order []peer.ID

	for _, relayStr := range PublicRelays {
		ma, err := multiaddr.NewMultiaddr(relayStr)
		if err != nil {
			continue
		}
		info, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		if existing, ok := merged[info.ID]; ok {
			existing.Addrs = append(existing.Addrs, info.Addrs...)
		} else {
			cp := *info
			merged[info.ID] = &cp
			order = append(order, info.ID)
		}
	}

	infos := make([]peer.AddrInfo, 0, len(order))
	for _, id := range order {
		infos = append(infos, *merged[id])
	}
	return infos
}
