# Security policy

Protocol v2 development is supported. Protocol v1 peers must upgrade; there is no automatic downgrade.

Report vulnerabilities privately through GitHub's private vulnerability reporting feature when available, or contact the maintainer listed in the repository profile. Do not publish secret material or exploit details in a public issue before coordination. Include the affected revision, reproduction steps, impact and any suggested fix.

## Intended guarantees

- Temporary, random peer identities and explicit connection-request acceptance.
- Encrypted transport and application packets; the handshake binds the request, transport peers and application public keys.
- Per-message symmetric key chains, packet authentication and replay/order rejection.
- Bounded file reception, whole-file integrity checks and explicit saving/overwrite consent.
- No persistent application chat database, configuration or disk debug log.
- Best-effort clearing of owned application key/file byte buffers on orderly close.

## Limits

Names are public discovery labels and are not globally unique or proofs of a person's identity. Fingerprints and transcript comparison words are optional; independent trusted comparison is needed for person-level verification. No trusted name directory, persistent trust relationship or anonymity mechanism is provided.

The cipher state uses symmetric key chains, not the complete Signal Double Ratchet; there is no in-session recovery from compromise of current chain keys. The application cryptography is not an independently audited implementation of Signal.

Public DHT and relay infrastructure can observe metadata and retain discovery records beyond a local session's lifetime. Disabling findability stops new advertisements and hides the live name but cannot erase existing network records immediately.

Go/runtime/library copies, swap, terminal capture, crash dumps, compromised devices, power loss and uncatchable termination prevent guarantees of complete memory erasure. User-requested file saving deliberately writes to disk and can leave a staging file after a crash. Unix mode 0600 does not promise equivalent Windows ACL protection. Debug stderr can be persisted by an external shell or supervisor.
