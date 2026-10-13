#!/usr/bin/env bash
# Build locally when possible; otherwise install a checksum-verified release.
set -euo pipefail
cd "$(dirname "$0")"
aegis_os=$(uname -s)
aegis_arch=$(uname -m)
case "$aegis_os" in Linux) aegis_platform=linux;; Darwin) aegis_platform=macos;; *) echo 'Unsupported operating system.' >&2; exit 1;; esac
case "$aegis_arch" in x86_64|amd64) aegis_arch=amd64;; aarch64|arm64) aegis_arch=arm64;; *) echo 'Unsupported architecture.' >&2; exit 1;; esac
aegis_tmp=$(mktemp -d)
trap 'rm -rf "$aegis_tmp"' EXIT

# Keep package-manager failures recoverable so the verified-release fallback runs.
if ! command -v go >/dev/null 2>&1; then
    echo 'Go is missing; trying the system package manager.'
    if [ "$aegis_platform" = macos ] && command -v brew >/dev/null 2>&1; then
        if ! brew install go; then echo 'Go installation failed; trying a release.' >&2; fi
    elif command -v apt-get >/dev/null 2>&1; then
        if ! (sudo apt-get update && sudo apt-get install -y golang); then echo 'Go installation failed; trying a release.' >&2; fi
    elif command -v dnf >/dev/null 2>&1; then
        if ! sudo dnf install -y golang; then echo 'Go installation failed; trying a release.' >&2; fi
    elif command -v pacman >/dev/null 2>&1; then
        if ! sudo pacman -S --noconfirm go; then echo 'Go installation failed; trying a release.' >&2; fi
    fi
fi

aegis_built=false
if command -v go >/dev/null 2>&1; then
    echo 'Building Aegis (Go 1.25.7 or newer required).'
    if go build -trimpath -o "$aegis_tmp/aegis" ./cmd/aegis; then aegis_built=true; fi
fi
if [ "$aegis_built" = false ]; then
    aegis_asset="aegis-${aegis_platform}-${aegis_arch}"
    aegis_release='https://github.com/rifatardarslan/Aegis/releases/latest/download'
    aegis_download() {
        if command -v curl >/dev/null 2>&1; then curl --fail --location --show-error --silent "$1" -o "$2"
        elif command -v wget >/dev/null 2>&1; then wget -q -O "$2" "$1"
        else echo 'curl or wget is required.' >&2; return 1; fi
    }
    echo "Downloading $aegis_asset and SHA256SUMS."
    aegis_download "$aegis_release/$aegis_asset" "$aegis_tmp/aegis"
    aegis_download "$aegis_release/SHA256SUMS" "$aegis_tmp/SHA256SUMS"
    aegis_expected=$(awk -v asset="$aegis_asset" '$2 == asset || $2 == "*"asset { print $1 }' "$aegis_tmp/SHA256SUMS")
    if [[ ! "$aegis_expected" =~ ^[0-9a-fA-F]{64}$ ]]; then echo 'Missing or invalid release checksum.' >&2; exit 1; fi
    if command -v sha256sum >/dev/null 2>&1; then aegis_actual=$(sha256sum "$aegis_tmp/aegis"); aegis_actual=${aegis_actual%% *}
    elif command -v shasum >/dev/null 2>&1; then aegis_actual=$(shasum -a 256 "$aegis_tmp/aegis"); aegis_actual=${aegis_actual%% *}
    else echo 'A SHA-256 utility is required.' >&2; exit 1; fi
    if [ "$aegis_actual" != "$aegis_expected" ]; then echo 'Release checksum mismatch; nothing installed.' >&2; exit 1; fi
fi

aegis_install_dir="${AEGIS_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$aegis_install_dir"
aegis_install_dir="$(cd "$aegis_install_dir" && pwd -P)"
install -m 0755 "$aegis_tmp/aegis" "$aegis_install_dir/aegis"
echo "Installed: $aegis_install_dir/aegis"
# An executed installer cannot change its parent terminal environment.
# Configure future shells, then explain how to pick up the change.
source ./scripts/configure-path.sh
aegis_profile_root="${AEGIS_PROFILE_DIR:-$HOME}"
aegis_shell_name="${SHELL:-/bin/sh}"
aegis_shell_name="${aegis_shell_name##*/}"
if [ "$aegis_shell_name" = zsh ] && [ -z "${AEGIS_PROFILE_DIR:-}" ]; then
    aegis_profile_root="${ZDOTDIR:-$HOME}"
fi
aegis_config_root="${AEGIS_CONFIG_DIR:-${XDG_CONFIG_HOME:-$aegis_profile_root/.config}}"
aegis_configure_path "$aegis_profile_root" "$aegis_install_dir" "${SHELL:-/bin/sh}" "$aegis_config_root"
echo 'Installation complete. Open a new terminal and run: aegis'
