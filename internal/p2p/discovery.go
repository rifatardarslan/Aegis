package p2p

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	circuitv2client "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	"github.com/multiformats/go-multiaddr"
	mh "github.com/multiformats/go-multihash"
)

// DefaultBootstrapPeers lists public IPFS bootstrap nodes to anchor into Kademlia DHT.
// Peer IDs and IPs verified 2026-06 via DNS TXT records for _dnsaddr.bootstrap.libp2p.io.
var DefaultBootstrapPeers = []string{
	// dnsaddr entries (resolves to all available transports)
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN", // sv15
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa", // ny5
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb", // am6
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt", // sg1
	// IP-based fallbacks for mobile networks where DNS may fail
	"/ip4/51.81.93.51/tcp/4001/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/ip4/51.81.93.51/udp/4001/quic-v1/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/ip4/147.135.44.132/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	"/ip4/147.135.44.132/udp/4001/quic-v1/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
}

// DiscoveryManager coordinates peer discovery using Kademlia DHT and mDNS.
type DiscoveryManager struct {
	DHT         *dht.IpfsDHT
	MDNS        mdns.Service
	host        host.Host
	customRelay string
}

type discoveryNotifee struct {
	h host.Host
}

func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.h.Peerstore().AddAddrs(pi.ID, pi.Addrs, time.Hour)
}

// NameCID maps a normalized name to a versioned discovery namespace.
func NameCID(name string) cid.Cid {
	hash := sha256.Sum256([]byte("aegis-name-v2:" + name))
	multihash, _ := mh.Encode(hash[:], mh.SHA2_256)
	return cid.NewCidV1(cid.Raw, multihash)
}

// NewDiscoveryManager sets up the DHT and mDNS discovery.
func NewDiscoveryManager(ctx context.Context, h host.Host, customRelay string) (*DiscoveryManager, error) {
	// Parse bootstrap addresses and pre-populate peerstore so DHT Bootstrap
	// has peers to query immediately when it starts.
	bootstrapInfos := parseBootstrapPeers(DefaultBootstrapPeers, customRelay)
	for _, info := range bootstrapInfos {
		h.Peerstore().AddAddrs(info.ID, info.Addrs, time.Hour)
	}

	// Initialize DHT in AutoServer mode: the node participates in DHT routing
	// when reachable (serves queries, stores records) and falls back to client
	// mode when behind restrictive NAT. Provider advertisements are used for
	// name discovery regardless of whether this node serves routing queries.
	kademliaDHT, err := dht.New(ctx, h,
		dht.Mode(dht.ModeAutoServer),
		dht.BootstrapPeers(bootstrapInfos...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create DHT: %w", err)
	}

	// Bootstrap triggers FIND_NODE queries — peerstore already populated above.
	if err := kademliaDHT.Bootstrap(ctx); err != nil {
		_ = kademliaDHT.Close()
		return nil, fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	dm := &DiscoveryManager{
		DHT:         kademliaDHT,
		host:        h,
		customRelay: customRelay,
	}

	// mDNS for same-LAN discovery.
	notifee := &discoveryNotifee{h: h}
	mdnsService := mdns.NewMdnsService(h, "aegis-p2p-mdns", notifee)
	if err := mdnsService.Start(); err != nil {
		// LAN discovery is optional; WAN discovery remains available.
	} else {
		dm.MDNS = mdnsService
	}

	// Connect to bootstrap peers at the host level (needed for relay).
	// Retry every 30s while routing table stays empty.
	go func() {
		dm.connectToBootstrap(ctx)
		// Re-trigger DHT bootstrap after host connections are established.
		_ = kademliaDHT.Bootstrap(ctx)
		// Proactively grab relay reservations so auto-relay can refresh them.
		dm.reserveRelays(ctx)

		bootstrapTicker := time.NewTicker(30 * time.Second)
		defer bootstrapTicker.Stop()

		relayTicker := time.NewTicker(5 * time.Minute)
		defer relayTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-bootstrapTicker.C:
				if dm.DHT.RoutingTable().Size() == 0 {
					dm.connectToBootstrap(ctx)
					_ = kademliaDHT.Bootstrap(ctx)
				}
			case <-relayTicker.C:
				if !hasRelayAddress(dm.host) {
					dm.reserveRelays(ctx)
				}
			}
		}
	}()

	return dm, nil
}

// StartAdvertising periodically refreshes this name while discoverability is enabled.
func (dm *DiscoveryManager) StartAdvertising(ctx context.Context, name string) {
	go func() {
		dm.waitForBootstrap(ctx, 30*time.Second)
		for ctx.Err() == nil {
			provideCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			_ = dm.DHT.Provide(provideCtx, NameCID(name), true)
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Minute):
			}
		}
	}()
}

// parseBootstrapPeers converts multiaddr strings (plus optional custom relay) into AddrInfos.
func parseBootstrapPeers(addrs []string, customRelay string) []peer.AddrInfo {
	all := make([]string, len(addrs))
	copy(all, addrs)
	if customRelay != "" {
		all = append(all, customRelay)
	}
	var infos []peer.AddrInfo
	for _, addrStr := range all {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			continue
		}
		info, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		infos = append(infos, *info)
	}
	return infos
}

// reserveRelays explicitly attempts circuit relay v2 RESERVE on each public relay node.
func (dm *DiscoveryManager) reserveRelays(ctx context.Context) {
	relayInfos := relayAddrInfos(dm.customRelay)
	var wg sync.WaitGroup
	for _, ri := range relayInfos {
		wg.Add(1)
		go func(info peer.AddrInfo) {
			defer wg.Done()
			rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			if err := dm.host.Connect(rctx, info); err != nil {
				return
			}
			_, _ = circuitv2client.Reserve(rctx, dm.host, info)
		}(ri)
	}
	wg.Wait()
}

func (dm *DiscoveryManager) connectToBootstrap(ctx context.Context) {
	infos := parseBootstrapPeers(DefaultBootstrapPeers, dm.customRelay)
	var wg sync.WaitGroup
	for _, info := range infos {
		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			bootstrapCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			_ = dm.host.Connect(bootstrapCtx, pi)
		}(info)
	}
	wg.Wait()
}

// FindPeer searches for the target PeerID using multiple strategies:
// 1. Local peerstore cache (covers mDNS and previously connected peers)
// 2. Direct DHT FindPeer lookup
func (dm *DiscoveryManager) FindPeer(ctx context.Context, targetID peer.ID) (peer.AddrInfo, error) {
	// 1. Check peerstore cache first (covers mDNS-discovered and previously connected peers).
	if addrs := dm.host.Peerstore().Addrs(targetID); len(addrs) > 0 {
		return peer.AddrInfo{ID: targetID, Addrs: addrs}, nil
	}

	// 2. If routing table is still empty, wait up to 15s for bootstrap to complete.
	if dm.DHT.RoutingTable().Size() == 0 {
		if !dm.waitForBootstrap(ctx, 15*time.Second) {
			return peer.AddrInfo{}, fmt.Errorf(
				"DHT not ready: no bootstrap connections — check internet access and firewall, then try again")
		}
		if addrs := dm.host.Peerstore().Addrs(targetID); len(addrs) > 0 {
			return peer.AddrInfo{ID: targetID, Addrs: addrs}, nil
		}
	}

	// 3. Direct DHT FindPeer lookup with 45s timeout.
	lookupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	pi, err := dm.DHT.FindPeer(lookupCtx, targetID)
	if err == nil {
		return pi, nil
	}

	return peer.AddrInfo{}, fmt.Errorf("peer not found in DHT: %w", err)
}

// waitForBootstrap polls the routing table until non-empty or timeout.
func (dm *DiscoveryManager) waitForBootstrap(ctx context.Context, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-tick.C:
			if dm.DHT.RoutingTable().Size() > 0 {
				return true
			}
		}
	}
}

// Close cleans up discovery resources.
func (dm *DiscoveryManager) Close() error {
	var errs []error
	if dm.MDNS != nil {
		if err := dm.MDNS.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if dm.DHT != nil {
		if err := dm.DHT.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to close discovery manager: %v", errs)
	}
	return nil
}

// hasRelayAddress reports whether the host has an active relay reservation address.
func hasRelayAddress(h host.Host) bool {
	for _, addr := range h.Addrs() {
		if strings.Contains(addr.String(), "/p2p-circuit") {
			return true
		}
	}
	return false
}
