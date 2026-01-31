#!/usr/bin/env bash
set -euo pipefail

# Install power-edge on a remote node via SSH
# Usage: ./install-remote.sh <ssh-destination> <node-config-dir>
# Example: ./install-remote.sh stella@10.8.0.1 config/nodes/stella-PowerEdge-T420
#
# Optional: Set SUDO_PASS environment variable for remote sudo password
# export SUDO_PASS=your-password

SSH_DEST="${1:-}"
NODE_CONFIG_DIR="${2:-}"

if [ -z "$SSH_DEST" ] || [ -z "$NODE_CONFIG_DIR" ]; then
    echo "Usage: $0 <ssh-destination> <node-config-dir>"
    echo "Example: $0 stella@10.8.0.1 config/nodes/stella-PowerEdge-T420"
    echo ""
    echo "Optional: Set SUDO_PASS for remote sudo password"
    echo "  export SUDO_PASS=your-password"
    exit 1
fi

if [ ! -d "$NODE_CONFIG_DIR" ]; then
    echo "❌ Error: Node config directory not found: $NODE_CONFIG_DIR"
    exit 1
fi

if [ ! -f "bin/power-edge-client" ]; then
    echo "❌ Error: Binary not found: bin/power-edge-client"
    echo "   Run 'make build-client' first"
    exit 1
fi

echo "🚀 Deploying power-edge to $SSH_DEST"
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

# Create systemd service
eval $SUDO_PREFIX tee /etc/systemd/system/power-edge.service > /dev/null << 'SERVICE'
[Unit]
Description=Power Edge - Edge State Controller
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
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log

[Install]
WantedBy=multi-user.target
SERVICE

# Reload systemd
eval $SUDO_PREFIX systemctl daemon-reload

# Enable and start service
eval $SUDO_PREFIX systemctl enable power-edge
eval $SUDO_PREFIX systemctl start power-edge

# Wait for service to start
sleep 2

# Check status
if eval $SUDO_PREFIX systemctl is-active --quiet power-edge; then
    echo "✅ Service started successfully"
    eval $SUDO_PREFIX systemctl status power-edge --no-pager
else
    echo "❌ Service failed to start"
    eval $SUDO_PREFIX journalctl -u power-edge -n 50 --no-pager
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
echo "   ssh $SSH_DEST sudo journalctl -u power-edge -f"
echo ""
echo "🛑 Stop service:"
echo "   ssh $SSH_DEST sudo systemctl stop power-edge"
echo ""
