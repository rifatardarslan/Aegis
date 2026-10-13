# Aegis

**Language / Dil:** [**English**](README.md) | [Türkçe](README_TR.md)

A terminal messenger for temporary, encrypted, one-to-one conversations and file sharing. Choose a temporary name, find the other person inside Aegis, and accept a connection request. No account, password, or external app is required to exchange connection addresses.

[Security policy](SECURITY.md) · MIT license

## Getting started

Build with **Go 1.25.7 or newer**:

```sh
git clone https://github.com/rifatardarslan/Aegis.git
cd Aegis
go run ./cmd/aegis
```

Or run `./install.sh` on Linux/macOS, or `install.bat` on Windows. Installers try a local build first and can fall back to a release containing an architecture-specific binary and `SHA256SUMS`. They fail if the download or checksum verification fails. Until such a release is published, install from source. Linux/macOS installs into `~/.local/bin` and adds it to your Bash, Zsh or Fish startup configuration. Open a new terminal after installation to run `aegis`. Other shells use `~/.profile`. Windows installs into `%USERPROFILE%\.aegis\bin` and updates the user PATH.

## A temporary conversation

1. Both people open Aegis and choose a name of 3–32 characters: letters, digits, `_` or `-`.
2. Search the other person's name inside Aegis. Matching is Unicode-normalized and case-insensitive. Names are not globally reserved; duplicates appear as separate peers with abbreviated temporary identities.
3. Select a person with the arrow keys and press Enter to send a request.
4. The recipient accepts with Y or rejects with N. A request expires after 60 seconds; Esc cancels an outgoing request or search.
5. Exchange encrypted messages and files. Esc disconnects; Ctrl+C exits. Unsaved received files must be saved or discarded before an interactive close completes.

F2 on the dashboard toggles findability. F3 opens advanced Peer ID/multiaddress connection. Search takes at most 30 seconds. Same-network discovery uses mDNS; internet discovery uses name-specific Kademlia provider records and existing libp2p bootstrap/relay nodes. Internet reachability depends on the network and relay availability. Turning off findability stops new advertisements and hides the live profile; old DHT records expire according to the network's own rules.

Each new conversation uses a new random libp2p identity and ephemeral application keys. The display name can remain in RAM while Aegis is open. There are no persistent accounts, contact lists, offline messages, or chat database.

## Files and shortcuts

| Shortcut | Action |
| --- | --- |
| Ctrl+F | Browse for a file |
| Ctrl+O | Enter a file path |
| Ctrl+S | Save a verified received file |
| Ctrl+X | Cancel a transfer or file preparation |
| F2 in chat | Show fingerprints and comparison words |
| Esc | Cancel the current dialog or disconnect |
| Ctrl+C | Exit, with an unsaved-file choice when needed |

Pasting or dragging an absolute file path into the chat input also offers that file.

`/help`, `/status`, `/clear`, `/disconnect`, and `/quit` are available in chat. Received acknowledgements indicate receipt, not that the person read the message. History is limited to 1,000 messages; reading older entries is not interrupted by new messages.

One file transfer at a time is supported, in either direction, up to **100 MB**, with 256 KB chunks. Transfers have independent identifiers, acceptance/ACK timeouts, cancellation and whole-file SHA-256 verification. The sender distinguishes transfer completion, receiver verification and saving. After completion or failure, another file can be transferred in the same conversation.

Received data remains in RAM until explicitly saved. A failed save keeps it available for retry. Existing files require overwrite confirmation; publication uses a private staging file in the chosen directory and does not truncate an existing destination in place. New files use owner-only permissions on Unix; Windows access follows the destination's filesystem/ACL rules. A process or OS crash during saving may leave the staging file.

## Security and limits

- libp2p authenticates transport peer identities. An X25519 exchange and Ed25519 signatures bind the application session to the request, both Peer IDs and ephemeral public keys.
- ChaCha20-Poly1305 encrypts packets with per-message HKDF-SHA256 key chains and monotonically checked counters. A single send worker serializes encryption and framing. Invalid packets terminate the connection.
- These are **symmetric key chains**, not the full Signal Double Ratchet. There is no DH ratchet or post-compromise recovery within a conversation.
- Names are discovery labels, **not proof of personal identity**. Fingerprints and six transcript-derived comparison words are optional and help only when independently compared through a trusted means. Checking them inside the same unverified connection is not independent identity verification.
- No application chat database, configuration file or disk debug log is created. `--debug` emits only fixed diagnostic categories to stderr; redirecting stderr is under the user's control.
- Owned application key and file byte buffers are cleared on orderly close. References to history and input are released. Go, terminal programs and operating systems may retain copies, swap, crash dumps or recordings; complete forensic erasure is not guaranteed.
- Discovery and relay infrastructure can observe connection metadata. Aegis does not provide anonymity or guarantee deletion of distributed discovery records. Uncatchable termination and compromised endpoints are outside the cleanup guarantees.

Protocol v2 uses `/aegis/2.0.0` and `/aegis/profile/1.0.0`. Both peers must update; there is no fallback to the old v1 conversation protocol.

```sh
aegis --relay '/ip4/RELAY_IP/tcp/PORT/p2p/RELAY_PEER_ID'
aegis --downloads ./received
aegis --debug
```

The custom relay is added as the preferred candidate; public bootstrap/relay infrastructure remains available.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/aegis
bash -n install.sh
```

`internal/p2p` owns discovery, requests and encrypted sessions. `internal/transfer` owns file state and saving. `internal/tui` owns interaction and tags asynchronous results with their session lifetime. Tests cover local peer connections, consent/cancellation, duplicate-name search, concurrent sends, framing, corrupt files, repeated transfers, safe saving and UI state changes. Real cross-network/CGNAT reachability needs testing on actual devices.
