#!/bin/bash

# Aegis Installer for Linux & macOS
# Automatically installs Go or downloads precompiled binaries if Go is missing.

set -e

# Change directory to the script's location
cd "$(dirname "$0")"

echo "==================================================="
echo "              🛡️ AEGIS INSTALLER 🛡️"
echo "==================================================="

# Detect OS
OS_TYPE="$(uname -s)"
ARCH_TYPE="$(uname -m)"

# 1. Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "[!] Go is not installed."
    
    # Try to install Go
    if [ "$OS_TYPE" = "Darwin" ]; then
        if command -v brew &> /dev/null; then
            echo "[!] Attempting to install Go via Homebrew..."
            brew install go
        else
            echo "[X] Homebrew not found. Cannot install Go automatically."
        fi
    elif [ "$OS_TYPE" = "Linux" ]; then
        if command -v apt-get &> /dev/null; then
            echo "[!] Attempting to install Go via apt-get..."
            sudo apt-get update && sudo apt-get install -y golang
        elif command -v dnf &> /dev/null; then
            echo "[!] Attempting to install Go via dnf..."
            sudo dnf install -y golang
        elif command -v pacman &> /dev/null; then
            echo "[!] Attempting to install Go via pacman..."
            sudo pacman -S --noconfirm go
        else
            echo "[X] No recognized package manager (apt, dnf, pacman) found."
        fi
    fi
fi

# 2. Check if Go is now available
if command -v go &> /dev/null; then
    echo "[!] Compiling Aegis from source with Go..."
    go install ./cmd/aegis
    
    # Locate output binary
    GOPATH_DIR=$(go env GOPATH)
    if [ -z "$GOPATH_DIR" ]; then
        GOPATH_DIR="$HOME/go"
    fi
    BIN_DIR="$GOPATH_DIR/bin"
    
    # Verify installation
    if [ -f "$BIN_DIR/aegis" ]; then
        chmod +x "$BIN_DIR/aegis"
        # Check if BIN_DIR is in PATH, if not, append to bashrc/zshrc
        if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
            echo "[!] Adding $BIN_DIR to PATH in shell profile..."
            if [ -f "$HOME/.zshrc" ]; then
                echo "export PATH=\$PATH:$BIN_DIR" >> "$HOME/.zshrc"
                echo "[i] Added to ~/.zshrc. Please run 'source ~/.zshrc' or open a new terminal."
            elif [ -f "$HOME/.bashrc" ]; then
                echo "export PATH=\$PATH:$BIN_DIR" >> "$HOME/.bashrc"
                echo "[i] Added to ~/.bashrc. Please run 'source ~/.bashrc' or open a new terminal."
            else
                echo "[i] Please add export PATH=\$PATH:$BIN_DIR to your shell profile."
            fi
        fi

        echo "==================================================="
        echo "[✔] AEGIS SUCCESSFULLY INSTALLED FROM SOURCE!"
        echo "==================================================="
        echo "[i] You can run Aegis by typing: aegis"
        echo "[i] (You might need to restart your terminal or run: source ~/.bashrc or source ~/.zshrc)"
        echo "==================================================="
        exit 0
    fi
fi

# 3. Fallback: Download precompiled binary from GitHub Releases
echo "[!] Go installation was not completed or build failed."
echo "[!] Attempting to download the latest precompiled binary from GitHub..."

REPO_URL="https://github.com/rifatardarslan/Aegis"
BINARY_NAME="aegis"

if [ "$OS_TYPE" = "Darwin" ]; then
    DOWNLOAD_URL="${REPO_URL}/releases/latest/download/aegis-macos"
elif [ "$OS_TYPE" = "Linux" ]; then
    DOWNLOAD_URL="${REPO_URL}/releases/latest/download/aegis-linux"
else
    echo "[X] Unsupported OS type: $OS_TYPE"
    exit 1
fi

# Download
if command -v curl &> /dev/null; then
    curl -L -o "$BINARY_NAME" "$DOWNLOAD_URL" || true
elif command -v wget &> /dev/null; then
    wget -O "$BINARY_NAME" "$DOWNLOAD_URL" || true
fi

# Verify local precompiled or downloaded binary exists
if [ ! -f "$BINARY_NAME" ]; then
    if [ -f "./aegis" ]; then
        BINARY_NAME="./aegis"
    else
        echo "[X] Could not find or download precompiled binary."
        echo "[X] Please install Go manually from https://go.dev/dl/"
        exit 1
    fi
fi

# Place binary in user bin directory
INSTALL_DIR="$HOME/.local/bin"
mkdir -p "$INSTALL_DIR"
cp "$BINARY_NAME" "$INSTALL_DIR/aegis"
chmod +x "$INSTALL_DIR/aegis"

# Clean up downloaded binary from root directory if it's there
if [ -f "./aegis" ] && [ "$INSTALL_DIR/aegis" != "./aegis" ]; then
    rm -f "./aegis"
fi

# Check if INSTALL_DIR is in PATH, if not, append to bashrc/zshrc
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo "[!] Adding $INSTALL_DIR to PATH in shell profile..."
    if [ -f "$HOME/.zshrc" ]; then
        echo "export PATH=\$PATH:$INSTALL_DIR" >> "$HOME/.zshrc"
        echo "[i] Added to ~/.zshrc. Please run 'source ~/.zshrc' or open a new terminal."
    elif [ -f "$HOME/.bashrc" ]; then
        echo "export PATH=\$PATH:$INSTALL_DIR" >> "$HOME/.bashrc"
        echo "[i] Added to ~/.bashrc. Please run 'source ~/.bashrc' or open a new terminal."
    else
        echo "[i] Please add export PATH=\$PATH:$INSTALL_DIR to your shell profile."
    fi
fi

echo "==================================================="
echo "[✔] AEGIS SUCCESSFULLY INSTALLED (PRECOMPILED BINARY)!"
echo "==================================================="
echo "[i] You can run Aegis by typing: aegis"
echo "[i] (You might need to restart your terminal or run: source ~/.bashrc or source ~/.zshrc)"
echo "==================================================="
