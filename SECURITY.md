# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.1.x   | ✅ Active  |

## Reporting a Vulnerability

If you discover a security vulnerability in Aegis, please report it responsibly.

**DO NOT** open a public GitHub issue for security vulnerabilities.

Instead, please send a detailed report via email or private message to the project maintainers. Include:

1. **Description** of the vulnerability
2. **Steps to reproduce** the issue
3. **Impact assessment** (what can an attacker do?)
4. **Suggested fix** (if you have one)

We will acknowledge receipt within 48 hours and provide a timeline for a fix.

## Security Design Principles

Aegis is built with the following security principles:

- **Zero-Trust Architecture**: Neither the network nor local storage is trusted
- **Memory-Only Operation**: No databases, logs, or config files are written to disk
- **Ephemeral Keys**: All cryptographic keys are session-scoped and never persisted
- **Forward Secrecy**: Double Ratchet protocol ensures per-message key derivation
- **Explicit Verification**: Out-of-band SAS and fingerprint comparison before chat

## Scope

The following are considered in-scope for security reports:

- Cryptographic implementation flaws
- Memory zeroization bypass
- Man-in-the-Middle attack vectors
- Replay or spoofing attack vectors
- Information disclosure through side channels
- Any mechanism that writes sensitive data to disk
