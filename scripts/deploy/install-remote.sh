#!/usr/bin/env bash
set -euo pipefail

# Install power-edge on a remote node via SSH
# Usage: ./install-remote.sh <ssh-destination> <node-config-dir>
# Example: ./install-remote.sh stella@10.8.0.1 data/nodes/stella-PowerEdge-T420
#
# Optional: Set SUDO_PASS environment variable for remote sudo password
# export SUDO_PASS=your-password

SSH_DEST="${1:-}"
NODE_CONFIG_DIR="${2:-}"

if [ -z "$SSH_DEST" ] || [ -z "$NODE_CONFIG_DIR" ]; then
    echo "Usage: $0 <ssh-destination> <node-config-dir>"
    echo "Example: $0 stella@10.8.0.1 data/nodes/stella-PowerEdge-T420"
    echo ""
    echo "Optional: Set SUDO_PASS for remote sudo password"
    echo "  export SUDO_PASS=your-password"
    exit 1
fi

if [ ! -d "$NODE_CONFIG_DIR" ]; then
    echo "❌ Error: Node config directory not found: $NODE_CONFIG_DIR"
    exit 1
fi

echo "🚀 Deploying power-edge to $SSH_DEST"
echo ""

# Detect target platform
echo "🔍 Detecting target platform..."
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLATFORM=$(bash "$SCRIPT_DIR/detect-platform.sh" "$SSH_DEST" "$NODE_CONFIG_DIR")

if [ -z "$PLATFORM" ]; then
    echo "❌ Failed to detect target platform"
    exit 1
fi

GOOS=$(echo "$PLATFORM" | cut -d'-' -f1)
GOARCH=$(echo "$PLATFORM" | cut -d'-' -f2)
echo "✅ Target platform: $PLATFORM (GOOS=$GOOS GOARCH=$GOARCH)"
echo ""

# Build binary for target platform
echo "🔨 Building power-edge-client for $PLATFORM..."
mkdir -p bin
if GOOS="$GOOS" GOARCH="$GOARCH" go build -ldflags "-X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') -X main.GitCommit=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.BuildTime=$(date -u '+%Y-%m-%d_%H:%M:%S')" -o bin/power-edge-client ./cmd/power-edge-client; then
    echo "✅ Build complete: bin/power-edge-client ($PLATFORM)"
else
    echo "❌ Build failed"
    exit 1
fi
echo ""

# Test connectivity
echo "Testing SSH connectivity..."
if ! ssh -o ConnectTimeout=5 "$SSH_DEST" "echo 'Connection OK'" >/dev/null 2>&1; then
    echo "❌ Cannot connect to $SSH_DEST"
    exit 1
fi
echo "✅ Connected"
echo ""

# Upload binary
echo "📦 Uploading binary..."
scp bin/power-edge-client "$SSH_DEST:/tmp/power-edge-client"
echo "✅ Binary uploaded"

# Upload configs
echo "📝 Uploading configs..."
ssh "$SSH_DEST" "mkdir -p /tmp/power-edge-configs"
scp "$NODE_CONFIG_DIR"/*.yaml "$SSH_DEST:/tmp/power-edge-configs/"
echo "✅ Configs uploaded"

# Install on remote node
echo "🔧 Installing power-edge on remote node..."

# Prepare sudo command based on whether password is provided
if [ -n "${SUDO_PASS:-}" ]; then
    echo "   Using sudo with password from SUDO_PASS"
    SUDO_CMD="echo \"\$SUDO_PASS\" | sudo -S"
else
    echo "   Using passwordless sudo (or will prompt)"
    SUDO_CMD="sudo"
fi

ssh "$SSH_DEST" "SUDO_PASS='${SUDO_PASS:-}'" 'bash -s' << 'REMOTE_INSTALL'
set -euo pipefail

# Setup sudo command
if [ -n "${SUDO_PASS:-}" ]; then
    SUDO_PREFIX="echo \"$SUDO_PASS\" | sudo -S"
else
    SUDO_PREFIX="sudo"
fi

# Create directories
eval $SUDO_PREFIX mkdir -p /etc/power-edge
eval $SUDO_PREFIX mkdir -p /usr/local/bin

# Move binary
eval $SUDO_PREFIX mv /tmp/power-edge-client /usr/local/bin/power-edge-client
eval $SUDO_PREFIX chmod +x /usr/local/bin/power-edge-client

# Move configs
eval $SUDO_PREFIX mv /tmp/power-edge-configs/*.yaml /etc/power-edge/
eval $SUDO_PREFIX chmod 644 /etc/power-edge/*.yaml

# Create service user (if not exists)
if ! id power-edge >/dev/null 2>&1; then
    eval $SUDO_PREFIX useradd --system --no-create-home --shell /bin/false power-edge
fi

# Configure passwordless sudo for specific commands
eval $SUDO_PREFIX tee /etc/sudoers.d/power-edge > /dev/null << 'SUDOERS'
# Power Edge reconciliation commands
power-edge ALL=(ALL) NOPASSWD: /usr/bin/systemctl
power-edge ALL=(ALL) NOPASSWD: /usr/sbin/sysctl
power-edge ALL=(ALL) NOPASSWD: /usr/sbin/ufw
power-edge ALL=(ALL) NOPASSWD: /usr/bin/apt-get
power-edge ALL=(ALL) NOPASSWD: /usr/bin/yum
power-edge ALL=(ALL) NOPASSWD: /usr/bin/dnf
SUDOERS

eval $SUDO_PREFIX chmod 440 /etc/sudoers.d/power-edge

# Create systemd template service
cat > /tmp/power-edge@.service << 'SERVICE'
[Unit]
Description=Power Edge Client (%i)
After=network.target

[Service]
Type=simple
User=power-edge
ExecStart=/usr/local/bin/power-edge-client \
  -state-config=/etc/power-edge/generated-state.yaml \
  -watcher-config=/etc/power-edge/generated-watcher-config.yaml \
  -listen=:9100
Restart=on-failure
RestartSec=10s

# Security hardening
# Note: NoNewPrivileges disabled to allow sudo for reconciliation
PrivateTmp=true
ProtectHome=true
ReadWritePaths=/var/log

[Install]
WantedBy=multi-user.target
SERVICE

# Move service file with sudo
eval $SUDO_PREFIX mv /tmp/power-edge@.service /etc/systemd/system/power-edge@.service
eval $SUDO_PREFIX chmod 644 /etc/systemd/system/power-edge@.service

# Reload systemd
eval $SUDO_PREFIX systemctl daemon-reload

# Enable and start service instance
eval $SUDO_PREFIX systemctl enable power-edge@client
eval $SUDO_PREFIX systemctl start power-edge@client

# Wait for service to start
sleep 2

# Check status
if eval $SUDO_PREFIX systemctl is-active --quiet power-edge@client; then
    echo "✅ Service started successfully"
    eval $SUDO_PREFIX systemctl status power-edge@client --no-pager
else
    echo "❌ Service failed to start"
    eval $SUDO_PREFIX journalctl -u power-edge@client -n 50 --no-pager
    exit 1
fi
REMOTE_INSTALL

echo ""
echo "✅ Deployment complete!"
echo ""

# Test endpoints
echo "🧪 Testing endpoints..."
REMOTE_IP=$(echo "$SSH_DEST" | cut -d'@' -f2)

for endpoint in /version /health /metrics; do
    echo -n "  $endpoint: "
    if ssh "$SSH_DEST" "curl -sf http://localhost:9100$endpoint" > /dev/null 2>&1; then
        echo "✅"
    else
        echo "❌"
    fi
done

echo ""
echo "📊 Metrics endpoint:"
echo "   http://$REMOTE_IP:9100/metrics"
echo ""
echo "🔍 View logs:"
echo "   ssh $SSH_DEST sudo journalctl -u power-edge@client -f"
echo ""
echo "🛑 Stop service:"
echo "   ssh $SSH_DEST sudo systemctl stop power-edge@client"
echo ""
