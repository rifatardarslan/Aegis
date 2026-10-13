package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const ProfileProtocol = "/aegis/profile/1.0.0"

type Profile struct {
	Name         string `json:"name"`
	PeerID       string `json:"peer_id"`
	Available    bool   `json:"available"`
	Discoverable bool   `json:"discoverable"`
}

type SearchResult struct {
	Profile Profile
	Peer    peer.AddrInfo
}

func NormalizeName(name string) (string, error) {
	name = norm.NFC.String(strings.TrimSpace(name))
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 3 || utf8.RuneCountInString(name) > 32 {
		return "", fmt.Errorf("name must contain 3–32 letters, digits, _ or -")
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return "", fmt.Errorf("name must contain only letters, digits, _ or -")
		}
	}
	return norm.NFC.String(cases.Fold().String(name)), nil
}

func (n *P2PNode) Profile() Profile {
	n.mu.Lock()
	defer n.mu.Unlock()
	return Profile{Name: n.name, PeerID: n.Host.ID().String(), Available: !n.busy && !n.closed, Discoverable: n.discoverable && !n.closed}
}

func (n *P2PNode) SetDiscoverable(enabled bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return
	}
	n.discoverable = enabled
	if n.advertiseCancel != nil {
		n.advertiseCancel()
		n.advertiseCancel = nil
	}
	if enabled && n.Discovery != nil {
		ctx, cancel := context.WithCancel(n.ctx)
		n.advertiseCancel = cancel
		normalized, _ := NormalizeName(n.name)
		n.Discovery.StartAdvertising(ctx, normalized)
	}
}

func (n *P2PNode) handleProfile(s network.Stream) {
	defer s.Close()
	if !n.profileLimiter.Allow() {
		_ = s.Reset()
		return
	}
	_ = s.SetDeadline(time.Now().Add(5 * time.Second))
	p := n.Profile()
	if !p.Discoverable {
		p.Name = ""
	}
	b, _ := json.Marshal(p)
	_ = WriteFrame(s, b)
}

func (n *P2PNode) QueryProfile(ctx context.Context, info peer.AddrInfo) (Profile, error) {
	if err := n.Host.Connect(ctx, info); err != nil {
		return Profile{}, err
	}
	s, err := n.Host.NewStream(network.WithAllowLimitedConn(ctx, "aegis profile"), info.ID, ProfileProtocol)
	if err != nil {
		return Profile{}, fmt.Errorf("profile unavailable or incompatible Aegis version: %w", err)
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(5 * time.Second))
	stop := context.AfterFunc(ctx, func() { _ = s.Reset() })
	defer stop()
	b, err := readControl(s)
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	if p.PeerID != info.ID.String() {
		return p, fmt.Errorf("profile does not match transport peer")
	}
	if _, err := NormalizeName(p.Name); err != nil {
		return p, err
	}
	return p, nil
}

// Search validates live profiles. Stale provider records never become live results.
func (n *P2PNode) Search(ctx context.Context, name string) ([]SearchResult, error) {
	normalized, err := NormalizeName(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	candidates := make(chan peer.AddrInfo, 64)
	go func() {
		defer close(candidates)
		for _, id := range n.Host.Peerstore().PeersWithAddrs() {
			if id == n.Host.ID() {
				continue
			}
			select {
			case candidates <- peer.AddrInfo{ID: id, Addrs: n.Host.Peerstore().Addrs(id)}:
			case <-ctx.Done():
				return
			}
		}
		if n.Discovery != nil {
			for pi := range n.Discovery.DHT.FindProvidersAsync(ctx, NameCID(normalized), 50) {
				select {
				case candidates <- pi:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[peer.ID]bool{}
	results := []SearchResult{}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pi := range candidates {
				mu.Lock()
				duplicate := seen[pi.ID]
				seen[pi.ID] = true
				mu.Unlock()
				if duplicate || pi.ID == n.Host.ID() {
					continue
				}
				checkCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				p, err := n.QueryProfile(checkCtx, pi)
				stop()
				if err != nil || !p.Discoverable {
					continue
				}
				key, err := NormalizeName(p.Name)
				if err != nil || key != normalized {
					continue
				}
				mu.Lock()
				results = append(results, SearchResult{Profile: p, Peer: pi})
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].Profile.PeerID < results[j].Profile.PeerID })
	if ctx.Err() != nil && len(results) == 0 {
		return results, fmt.Errorf("search ended: %w", ctx.Err())
	}
	return results, nil
}
