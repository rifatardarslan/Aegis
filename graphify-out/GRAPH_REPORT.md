# Graph Report - .  (2026-07-06)

## Corpus Check
- Corpus is ~24,326 words - fits in a single context window. You may not need a graph.

## Summary
- 253 nodes · 475 edges · 16 communities (13 shown, 3 thin omitted)
- Extraction: 80% EXTRACTED · 20% INFERRED · 0% AMBIGUOUS · INFERRED: 97 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- [[_COMMUNITY_TUI Banner & Dashboard|TUI Banner & Dashboard]]
- [[_COMMUNITY_Security Concepts & Docs|Security Concepts & Docs]]
- [[_COMMUNITY_App Entry & Chat Model|App Entry & Chat Model]]
- [[_COMMUNITY_File Transfer & Chat UI|File Transfer & Chat UI]]
- [[_COMMUNITY_P2P Discovery & NAT|P2P Discovery & NAT]]
- [[_COMMUNITY_P2P Node & Rate Limiting|P2P Node & Rate Limiting]]
- [[_COMMUNITY_Identity & Handshake|Identity & Handshake]]
- [[_COMMUNITY_Double Ratchet & AEAD Crypto|Double Ratchet & AEAD Crypto]]
- [[_COMMUNITY_Host Key & Stream Framing|Host Key & Stream Framing]]
- [[_COMMUNITY_SAS Verification TUI|SAS Verification TUI]]
- [[_COMMUNITY_Relay Bootstrap|Relay Bootstrap]]
- [[_COMMUNITY_Frame Protocol|Frame Protocol]]
- [[_COMMUNITY_Installer Script|Installer Script]]
- [[_COMMUNITY_Run Script|Run Script]]
- [[_COMMUNITY_Module Root|Module Root]]

## God Nodes (most connected - your core abstractions)
1. `P2PNode` - 29 edges
2. `DashboardModel` - 17 edges
3. `ChatModel` - 16 edges
4. `Bytes()` - 14 edges
5. `DiscoveryManager` - 13 edges
6. `Aegis Landing Page` - 12 edges
7. `Array32()` - 11 edges
8. `Model` - 11 edges
9. `NewDiscoveryManager()` - 10 edges
10. `RunHandshake()` - 9 edges

## Surprising Connections (you probably didn't know these)
- `Aegis Landing Page` --semantically_similar_to--> `Aegis`  [INFERRED] [semantically similar]
  docs/index.html → README.md
- `Aegis (Türkçe Dokümantasyon)` --semantically_similar_to--> `Aegis`  [INFERRED] [semantically similar]
  README_TR.md → README.md
- `Security Design Principles` --semantically_similar_to--> `Zero-Trust Threat Model`  [INFERRED] [semantically similar]
  SECURITY.md → README.md
- `main()` --calls--> `NewModel()`  [INFERRED]
  cmd/aegis/main.go → internal/tui/model.go
- `Go Cross-Platform CI Pipeline` --references--> `Aegis`  [INFERRED]
  .github/workflows/go.yml → README.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Authenticated Key Exchange & Ratchet Bootstrap** — readme_argon2id_derivation, readme_x25519_handshake, readme_ed25519_signatures, readme_double_ratchet [INFERRED 0.85]
- **Out-of-Band Anti-MitM Verification** — readme_oob_verification, readme_sas, readme_fingerprint, readme_bip39_wordlist [EXTRACTED 0.75]
- **Decentralized P2P Transport Stack** — readme_kademlia_dht, readme_mdns_discovery, readme_nat_hole_punching, readme_relay_fallback, readme_yamux_transport [EXTRACTED 0.75]

## Communities (16 total, 3 thin omitted)

### Community 0 - "TUI Banner & Dashboard"
Cohesion: 0.12
Nodes (24): RenderBanner(), ScanBar(), check(), clearClipboardCmd(), connectP2PCmd(), fmtDur(), Cmd, Duration (+16 more)

### Community 1 - "Security Concepts & Docs"
Cohesion: 0.08
Nodes (33): Aegis Landing Page, Animated Terminal Demo, Go Cross-Platform CI Pipeline, Cross-Platform Build & Test Matrix, Aegis, Passphrase-Derived Host Key (Argon2id), BIP-39 Wordlist, Cascading Zeroization (+25 more)

### Community 2 - "App Entry & Chat Model"
Cohesion: 0.11
Nodes (22): main(), NewChat(), NewDashboard(), LogDebug(), Cmd, Msg, Stream, listenForIncomingStreams() (+14 more)

### Community 3 - "File Transfer & Chat UI"
Cohesion: 0.12
Nodes (20): SaveFile(), HashFile(), T, TestHashFile(), TestSaveFilePermissions(), Cmd, Duration, Msg (+12 more)

### Community 4 - "P2P Discovery & NAT"
Cohesion: 0.15
Nodes (16): Cid, AddrInfo, Context, Duration, Host, ID, hasRelayAddress(), makeRendezvousCID() (+8 more)

### Community 5 - "P2P Node & Rate Limiting"
Cohesion: 0.12
Nodes (15): CancelFunc, Context, Host, ID, PublicKey, Stream, Time, isUsableIP() (+7 more)

### Community 6 - "Identity & Handshake"
Cohesion: 0.19
Nodes (14): HandshakeResult, Identity, T, TestCryptoKeysZeroization(), TestHandshakeAndRatchet(), TestZeroize(), PublicKey, RunHandshake() (+6 more)

### Community 7 - "Double Ratchet & AEAD Crypto"
Cohesion: 0.23
Nodes (10): RatchetState, Open(), Seal(), kdfStep(), makeNonce(), NewRatchetState(), Array32(), Bytes() (+2 more)

### Community 8 - "Host Key & Stream Framing"
Cohesion: 0.16
Nodes (12): DeriveHostKey(), Context, Host, NewHost(), T, TestDeriveHostKey(), TestStreamFraming(), ReadFrame() (+4 more)

### Community 9 - "SAS Verification TUI"
Cohesion: 0.29
Nodes (4): Cmd, Msg, NewVerify(), VerifyModel

### Community 10 - "Relay Bootstrap"
Cohesion: 0.67
Nodes (3): AddrInfo, main(), mergedRelays()

### Community 11 - "Frame Protocol"
Cohesion: 0.67
Nodes (3): AegisFrame, FrameType, p2pFileFrameMsg

## Knowledge Gaps
- **25 isolated node(s):** `github.com/aegis-p2p/aegis`, `install.sh script`, `p2pChunkSentSuccessMsg`, `p2pChunkSendErrorMsg`, `scanStepMsg` (+20 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **3 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `P2PNode` connect `P2P Node & Rate Limiting` to `TUI Banner & Dashboard`, `App Entry & Chat Model`, `File Transfer & Chat UI`, `P2P Discovery & NAT`, `Identity & Handshake`, `Double Ratchet & AEAD Crypto`, `SAS Verification TUI`?**
  _High betweenness centrality (0.352) - this node is a cross-community bridge._
- **Why does `DashboardModel` connect `TUI Banner & Dashboard` to `App Entry & Chat Model`, `P2P Node & Rate Limiting`?**
  _High betweenness centrality (0.115) - this node is a cross-community bridge._
- **Why does `DiscoveryManager` connect `P2P Discovery & NAT` to `P2P Node & Rate Limiting`?**
  _High betweenness centrality (0.108) - this node is a cross-community bridge._
- **Are the 13 inferred relationships involving `Bytes()` (e.g. with `.Destroy()` and `Open()`) actually correct?**
  _`Bytes()` has 13 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/aegis-p2p/aegis`, `install.sh script`, `p2pChunkSentSuccessMsg` to the rest of the system?**
  _25 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `TUI Banner & Dashboard` be split into smaller, more focused modules?**
  _Cohesion score 0.11806543385490754 - nodes in this community are weakly interconnected._
- **Should `Security Concepts & Docs` be split into smaller, more focused modules?**
  _Cohesion score 0.08143939393939394 - nodes in this community are weakly interconnected._