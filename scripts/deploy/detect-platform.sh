#!/usr/bin/env bash
set -euo pipefail

# Detect target platform for cross-compilation
# Usage: ./detect-platform.sh <ssh-destination> [node-config-dir]
# Returns: GOOS-GOARCH (e.g., linux-amd64, darwin-arm64)

SSH_DEST="${1:-}"
NODE_CONFIG_DIR="${2:-}"

if [ -z "$SSH_DEST" ]; then
    echo "Usage: $0 <ssh-destination> [node-config-dir]" >&2
    exit 1
fi

# Try to read from discovery data first (fastest)
if [ -n "${NODE_CONFIG_DIR:-}" ] && [ -f "$NODE_CONFIG_DIR/discovery-data/system-info.json" ]; then
    OS_TYPE=$(jq -r '.os_type // empty' "$NODE_CONFIG_DIR/discovery-data/system-info.json" 2>/dev/null || echo "")
    ARCH=$(jq -r '.architecture // empty' "$NODE_CONFIG_DIR/discovery-data/system-info.json" 2>/dev/null || echo "")

    if [ -n "$OS_TYPE" ] && [ -n "$ARCH" ]; then
        # Map to Go platform names
        case "$OS_TYPE" in
            linux) GOOS="linux" ;;
            darwin) GOOS="darwin" ;;
            *) GOOS="" ;;
        esac

        case "$ARCH" in
            x86_64|amd64) GOARCH="amd64" ;;
            aarch64|arm64) GOARCH="arm64" ;;
            armv7l|armhf) GOARCH="arm" ;;
            i386|i686) GOARCH="386" ;;
            *) GOARCH="" ;;
        esac

        if [ -n "$GOOS" ] && [ -n "$GOARCH" ]; then
            echo "${GOOS}-${GOARCH}"
            exit 0
        fi
    fi
fi

# Fallback: SSH and detect
OS_TYPE=$(ssh "$SSH_DEST" "uname -s" 2>/dev/null | tr '[:upper:]' '[:lower:]')
ARCH=$(ssh "$SSH_DEST" "uname -m" 2>/dev/null)

# Map to Go platform names
case "$OS_TYPE" in
    linux) GOOS="linux" ;;
    darwin) GOOS="darwin" ;;
    freebsd) GOOS="freebsd" ;;
    openbsd) GOOS="openbsd" ;;
    netbsd) GOOS="netbsd" ;;
    *)
        echo "ERROR: Unsupported OS: $OS_TYPE" >&2
        exit 1
        ;;
esac

case "$ARCH" in
    x86_64|amd64) GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    armv7l|armhf) GOARCH="arm" ;;
    i386|i686) GOARCH="386" ;;
    *)
        echo "ERROR: Unsupported architecture: $ARCH" >&2
        exit 1
        ;;
esac

echo "${GOOS}-${GOARCH}"
