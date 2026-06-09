package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	circuitv2client "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/multiformats/go-multiaddr"
)

// Updated relay list — verified reachable as of 2026-06
var relayStrs = []string{
	// 104.131.131.82 (old va1) — still up
	"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGjV7zav2qqDZ45NS7K2zEtPA87qIS6iND4G1RS6",
	"/ip4/104.131.131.82/udp/4001/quic-v1/p2p/QmaCpDMGjV7zav2qqDZ45NS7K2zEtPA87qIS6iND4G1RS6",
	// sv15 (147.135.44.132) — new peer ID
	"/ip4/147.135.44.132/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	"/ip4/147.135.44.132/udp/4001/quic-v1/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	// ny5 (51.81.93.51)
	"/ip4/51.81.93.51/tcp/4001/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/ip4/51.81.93.51/udp/4001/quic-v1/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	// am6 (54.38.47.166) — new peer ID
	"/ip4/54.38.47.166/tcp/4001/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	"/ip4/54.38.47.166/udp/4001/quic-v1/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	// sg1 (15.235.144.210) — new peer ID
	"/ip4/15.235.144.210/tcp/4001/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
	"/ip4/15.235.144.210/udp/4001/quic-v1/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
}

func mergedRelays() []peer.AddrInfo {
	byID := make(map[peer.ID]*peer.AddrInfo)
	var order []peer.ID
	for _, s := range relayStrs {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			fmt.Printf("[parse] FAIL %s: %v\n", s, err)
			continue
		}
		info, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			fmt.Printf("[parse] FAIL addrinfo %s: %v\n", s, err)
			continue
		}
		if ex, ok := byID[info.ID]; ok {
			ex.Addrs = append(ex.Addrs, info.Addrs...)
		} else {
			cp := *info
			byID[info.ID] = &cp
			order = append(order, info.ID)
		}
	}
	out := make([]peer.AddrInfo, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

func main() {
	ctx := context.Background()

	relayInfos := mergedRelays()
	fmt.Printf("Relay nodes (merged): %d\n\n", len(relayInfos))
	for _, r := range relayInfos {
		fmt.Printf("  %s  addrs=%d\n", r.ID, len(r.Addrs))
	}
	fmt.Println()

	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0", "/ip4/0.0.0.0/udp/0/quic-v1"),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),
		libp2p.EnableAutoRelayWithStaticRelays(relayInfos),
		libp2p.EnableNATService(),
		libp2p.ForceReachabilityPrivate(),
	)
	if err != nil {
		fmt.Printf("ERROR creating host: %v\n", err)
		return
	}
	defer h.Close()

	fmt.Printf("Host ID: %s\n\n", h.ID())

	// Connect to each relay and try explicit circuit relay v2 reservation
	for _, r := range relayInfos {
		go func(ri peer.AddrInfo) {
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			if err := h.Connect(cctx, ri); err != nil {
				fmt.Printf("[connect] FAIL %s: %v\n", ri.ID.ShortString(), err)
				return
			}
			fmt.Printf("[connect] OK   %s\n", ri.ID.ShortString())

			// Try explicit circuit relay v2 RESERVE
			rctx, rcancel := context.WithTimeout(ctx, 15*time.Second)
			defer rcancel()
			reservation, err := circuitv2client.Reserve(rctx, h, ri)
			if err != nil {
				fmt.Printf("[reserve] FAIL %s: %v\n", ri.ID.ShortString(), err)
			} else {
				fmt.Printf("[reserve] OK   %s  expire=%s\n", ri.ID.ShortString(), reservation.Expiration.Format(time.RFC3339))
			}
		}(r)
	}

	start := time.Now()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	deadline := time.After(60 * time.Second)

	for {
		select {
		case <-deadline:
			fmt.Printf("\n[%.0fs] DONE\n", time.Since(start).Seconds())
			return
		case <-ticker.C:
			elapsed := time.Since(start).Seconds()
			addrs := h.Addrs()
			var circuit []string
			for _, a := range addrs {
				if strings.Contains(a.String(), "p2p-circuit") {
					circuit = append(circuit, a.String())
				}
			}
			peers := h.Network().Peers()
			fmt.Printf("[%.0fs] peers=%d  circuit_addrs=%d\n", elapsed, len(peers), len(circuit))
			for _, ca := range circuit {
				fmt.Printf("  RELAY: %s\n", ca)
			}
		}
	}
}
