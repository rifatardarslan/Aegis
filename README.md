# 🛡️ Aegis

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-informational)](.)

📖 **Türkçe readme için tıklayın:** [🇹🇷 Türkçe](./README_TR.md)

> **Zero-Trust · Memory-Only · P2P Secure Terminal Messenger**
> 
> Aegis is a military-grade, zero-storage peer-to-peer secure messaging and file transfer tool built entirely for the terminal (TUI) with a sleek cyberpunk/hacker aesthetic.

---

## 📂 Project Structure

```
aegis/
├── cmd/
│   └── aegis/
│       └── main.go              # Application entry point & signal handling
├── internal/
│   ├── crypto/
│   │   ├── aead.go              # ChaCha20-Poly1305 Seal/Open
│   │   ├── handshake.go         # X25519 DH + Ed25519 mutual authentication
│   │   ├── identity.go          # Ephemeral Ed25519 keypair & SAS generation
│   │   ├── ratchet.go           # Double Ratchet engine (HKDF-SHA256 KDF chain)
│   │   ├── wordlist.go          # BIP-39 wordlist for SAS display
│   │   ├── zeroize.go           # unsafe memory scrubbing (Bytes/Array32/String)
│   │   └── crypto_test.go       # Cryptographic unit tests
│   ├── p2p/
│   │   ├── host.go              # Argon2id key derivation & libp2p host init
│   │   ├── node.go              # P2PNode: connect, handshake, lifecycle
│   │   ├── discovery.go         # Kademlia DHT + mDNS peer discovery
│   │   ├── nat.go               # Public relay addresses for NAT traversal
│   │   ├── stream.go            # Length-prefixed framing (ReadFrame/WriteFrame)
│   │   └── node_test.go         # P2P unit tests
│   ├── transfer/
│   │   ├── frame.go             # AegisFrame JSON protocol definition
│   │   ├── sender.go            # Streaming SHA-256 file hashing
│   │   ├── receiver.go          # Secure file save (0600 permissions)
│   │   └── transfer_test.go     # Transfer unit tests
│   └── tui/
│       ├── model.go             # Root Bubbletea model & screen routing
│       ├── dashboard.go         # Screen 1: passphrase entry & peer connect
│       ├── verify.go            # Screen 2: fingerprint & SAS verification
│       ├── chat.go              # Screen 3: encrypted chat & file transfer
│       ├── styles.go            # Lipgloss color palette & style constants
│       └── banner.go            # ASCII art banner renderer
├── install.bat                  # One-click Windows installer (WDAC bypass)
├── run.sh                       # One-step Linux/macOS launcher
├── Makefile                     # Build, install, dev, debug, vet targets
├── go.mod / go.sum              # Go module dependencies
├── LICENSE                      # MIT License
├── SECURITY.md                  # Security vulnerability reporting policy
├── README.md                    # Documentation (English)
└── README_TR.md                 # Documentation (Türkçe)
```

---

## 🔒 Security Architecture & Cryptography Layer

Aegis is built with a zero-trust threat model, treating both the transport network and persistent local storage as untrusted domains.

### 1. Ephemeral & Zero-Storage Promise
* **Strict RAM Boundaries:** No databases, no logs, and no local configuration files are ever created. Cryptographic keys, session transcripts, message history, and incoming/outgoing file buffers reside strictly in volatile RAM.
* **Cascading Zeroization:** Active cryptographic keys (Double Ratchet root and chain keys, ephemeral Ed25519 identity keys) and file buffers are explicitly zero-scrubbed in memory using direct pointer mutation (`unsafe.StringData` and `unsafe.Slice`) combined with `runtime.KeepAlive` compiler barriers immediately upon regular exit, network disconnections, or OS signals.

### 2. Ephemeral Identity & Authenticated Key Exchange
* **Passphrase-Derived Host Key:** On startup, Aegis prompts for a master passphrase, deriving a deterministic libp2p host key using Argon2id with strict parameters (3 iterations, 64 MB RAM, 4 threads). The passphrase is zeroized from memory immediately after derivation at the boundary layer.
* **X25519 DH Handshake:** Aegis implements a manual ephemeral X25519 Diffie-Hellman exchange to derive a shared session root key. The shared secret is zeroed via `defer` immediately after HKDF expansion.
* **Ed25519 Identity Signatures:** Key exchanges are authenticated via an ephemeral Ed25519 keypair generated dynamically per session. Handshake transcripts are verified using signatures to eliminate active Man-in-the-Middle (MitM) positioning.

### 3. Symmetric Double Ratchet Engine
* **Perfect Forward Secrecy:** Messages are encrypted using a customized implementation of the Signal Double Ratchet protocol. Each sent and received message derives a unique, temporary key using HKDF-SHA256, advancing the chain. KDF intermediate buffers and derived message keys are zeroed via `defer` after each step.
* **ChaCha20-Poly1305 AEAD:** Payload messages are sealed and authenticated using standard ChaCha20-Poly1305. The AEAD key is zeroed after each encryption/decryption. Nonces are constructed using monotonic message counters, and Associated Data (AAD) binds remote Peer IDs lexicographically to prevent replay or spoofing attacks.

### 4. Out-of-Band (OOB) Verification (Anti-MitM)
* **Fingerprint Comparison:** Aegis displays the SSH-style SHA-256 base64 public identity key fingerprints on a verification screen upon connection.
* **Short Authentication String (SAS):** Displays a symmetric 3-word SAS derived from the XOR of public key hashes mapped to the standard BIP-39 wordlist. Users confirm matches verbally or via a secondary trusted channel before proceeding to chat.

---

## 📂 Secure Multiplexed File Transfer

Aegis supports high-speed, secure, memory-only file transfers multiplexed over the same active `/aegis/1.0.0` libp2p stream.

* **Drag-and-Drop / Paste Path:** Drag a file directly from your operating system explorer into the terminal window (or paste/type the absolute path) and hit Enter to instantly initiate a secure transfer.
* **Stream Hash & Chunking:** Files are read in a streaming fashion, divided into standard `256 KB` chunks, and individually hashed using SHA-256.
* **RAM Assembly:** Received chunks are collected directly in-RAM. No temporary files are written to disk.
* **Integrity Verification:** Upon completion, the receiver independently computes the full SHA-256 hash and compares it to the sender's advertised hash. Mismatches trigger an immediate rejection and buffer zeroization.
* **Owner-Only Permissions:** Verified in-memory file buffers are written to disk once with strict `0600` owner-only permissions.
* **Sleek TUI Feedback:** Transfers feature a live progress bar rendered in the system status panel alongside chunk validation diagnostics.

---

## 🛠️ P2P Routing & NAT Traversal

Aegis operates in a completely decentralized topology without central messaging servers.

* **Transport & Muxing:** Runs over secure transport streams (TCP / QUIC) multiplexed using Yamux.
* **Discovery:** Resolves peers globally using Kademlia DHT bootstrap anchoring and LAN peers via multicast DNS (mDNS).
* **NAT Hole-Punching:** Resolves complex firewalls via AutoNAT and DCUtR hole punching protocols.
* **Relay Fallback:** Supports automatic circuit relays utilizing public IPFS bootstrap nodes, with optional custom relay configuration via CLI flag `--relay`.

---

## 🚀 Getting Started & Installation Guide

Aegis supports fully automated, single-command installation across Windows, Linux, and macOS. The installation scripts automatically check for Go, install it via system package managers (winget, apt-get, Homebrew, dnf, pacman) if missing, or download precompiled release binaries if Go cannot be compiled.

### Prerequisites
* **Git** (for cloning the repository).
* **Go 1.21+** is recommended (but will be automatically installed by the script if missing).

```bash
# Clone the repository
git clone https://github.com/rifatardarslan/Aegis.git
cd aegis
```

---

### 💻 1. Windows Installation (Automated)

To install Aegis on Windows, simply run the installation script in the root directory:

```powershell
# Double-click 'install.bat' or run it via terminal:
.\install.bat
```

**What This Installer Does:**
1. **Go Environment Check:** Checks if Go is installed on your system.
2. **Auto-Install (winget):** If Go is missing, it automatically attempts to install it via Windows Package Manager (`winget`).
3. **Precompiled Fallback:** If Go installation is skipped or fails, it downloads the latest precompiled `aegis.exe` from GitHub Releases.
4. **Environment Configuration:** Places the binary and launcher wrapper in your PATH (`GOPATH\bin` or `~/.aegis/bin`) so you can run it globally.

**Running:**
Once completed, open a **new** Command Prompt or PowerShell window and type:
```powershell
aegis
```

---

### 🐧 2. Linux & macOS Installation (Automated)

For Linux (Kali, Ubuntu, Debian) and macOS, run the automated installation script:

```bash
# Run the installation script:
chmod +x install.sh
./install.sh
```

**What This Installer Does:**
1. **Go Environment Check:** Checks if Go is installed on your system.
2. **Auto-Install:** If Go is missing, it attempts to install it via your package manager (`apt-get`, `brew`, `dnf`, or `pacman`).
3. **Precompiled Fallback:** If Go installation fails or is skipped, it downloads the corresponding precompiled release binary (`aegis-linux` or `aegis-macos`) from GitHub Releases.
4. **Environment Configuration:** Installs the binary to `$HOME/.local/bin` and updates your shell profile (`.bashrc` or `.zshrc`) if necessary.

**Running:**
Once completed, restart your terminal or run `source ~/.bashrc` (or `source ~/.zshrc`) and type:
```bash
aegis
```

---

### ⚙️ Command-Line Parameters (All Platforms)
Use these flags when launching Aegis to adjust its behavior:

| Flag | Description |
|------|-------------|
| `--debug` | Enable advanced network operational logging (cryptographic keys are **never** logged) |
| `--relay "<multiaddr>"` | Bind to a custom circuit relay multiaddr instead of default IPFS bootstrap nodes |

**Examples:**
```bash
aegis --debug
aegis --relay "/ip4/1.2.3.4/tcp/4001/p2p/Qm..."
```

---

## 📡 Live Execution Walkthrough

1. **Host Setup:** Two instances are launched on separate terminals via `aegis`.
2. **Identity Derivation:** Both users enter their private master passphrase. The Libp2p Peer ID is deterministically established via Argon2id.
3. **Connection:** One user inputs the remote peer's Peer ID in the connect input field. Kademlia DHT initiates routing and NAT hole punching.
4. **Verification Screen:** A warning panel flashes. Compare the public fingerprints and BIP-39 SAS (e.g., `DELTA · ECHO · FOXTROT`). Press **Y** to proceed or **N** to abort.
5. **Secure Chat:** Start sending secure messages. Double-ratchet numbers advance in the system panel.
6. **File Transfer:** Drag-and-drop a file directly into the terminal text input box (or press **Ctrl+F** to use the file browser) and press **ENTER**. The receiver accepts the offer via **Y**. Watch the progress bar complete. Save the file securely by pressing **Ctrl+S** and specifying the output directory or path (Aegis automatically appends the filename if a directory is given).
7. **Secure Tear-Down:** Press **Ctrl+C** or close the terminal. All cryptographic material and RAM buffers are zeroized instantly.

---

## 🛡️ Threat Model Mitigations

| Threat | Security Mitigation |
|---|---|
| **Passive Network Eavesdropping** | Double-encrypted wire transport utilizing Yamux streams over TLS 1.3 QUIC and ChaCha20-Poly1305. |
| **Active Man-in-the-Middle (MitM)** | Out-of-band fingerprint matching and 3-word Short Authentication String (SAS) transcript checking combined with Ed25519 stream signing. |
| **Replay or Spoofing Attacks** | Monotonic packet counters bound to unique AEAD nonces. Unsigned or duplicate sequence packets are rejected. |
| **Key Compromise / Leakage** | Ephemeral Ed25519 signing keys. Double Ratchet engine wipes previous message keys immediately upon receipt (PFS). |
| **Cold-Boot Memory Forensics** | Direct unsafe memory zeroization of raw private keys, derived matrices, and file bytes on any exit, disconnection, or interrupt. |
| **Passphrase Exposure** | User passphrases are never written to disk, and the stack/heap buffers are scrubbed at the boundary layer immediately after deterministic key derivation. |

---

## 📜 License

This project is licensed under the [MIT License](LICENSE).
