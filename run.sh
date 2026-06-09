#!/bin/bash
# Aegis cross-platform quick launcher for Linux (Kali) and macOS
# Usage: ./run.sh [--debug] [--relay <addr>]

# Change directory to the script's folder to ensure go.mod resolves correctly
cd "$(dirname "$0")"

go run ./cmd/aegis "$@"
