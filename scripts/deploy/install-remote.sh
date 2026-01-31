#!/usr/bin/env bash
set -euo pipefail

# Install power-edge on a remote node via SSH
# Usage: ./install-remote.sh <ssh-host> <node-config-dir>
# Example: ./install-remote.sh stella@10.8.0.1 config/nodes/stella-PowerEdge-T420

SSH_HOST="${1:-}"
NODE_CONFIG_DIR="${2:-}"

if [ -z "$SSH_HOST" ] || [ -z "$NODE_CONFIG_DIR" ]; then
    echo "Usage: $0 <ssh-host> <node-config-dir>"
    echo "Example: $0 stella@10.8.0.1 config/nodes/stella-PowerEdge-T420"
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

echo "🚀 Deploying power-edge to $SSH_HOST"
echo ""

# Test connectivity
echo "Testing SSH connectivity..."
if ! ssh -o ConnectTimeout=5 "$SSH_HOST" "echo 'Connection OK'" >/dev/null 2>&1; then
    echo "❌ Cannot connect to $SSH_HOST"
    exit 1
fi
echo "✅ Connected"
echo ""

# Upload binary
echo "📦 Uploading binary..."
scp bin/power-edge-client "$SSH_HOST:/tmp/power-edge-client"
echo "✅ Binary uploaded"

# Upload configs
echo "📝 Uploading configs..."
ssh "$SSH_HOST" "mkdir -p /tmp/power-edge-configs"
scp "$NODE_CONFIG_DIR"/*.yaml "$SSH_HOST:/tmp/power-edge-configs/"
echo "✅ Configs uploaded"

# Install on remote node
echo "🔧 Installing power-edge on remote node..."
ssh "$SSH_HOST" 'bash -s' << 'REMOTE_INSTALL'
set -euo pipefail

# Create directories
sudo mkdir -p /etc/power-edge
sudo mkdir -p /usr/local/bin

# Move binary
sudo mv /tmp/power-edge-client /usr/local/bin/power-edge-client
sudo chmod +x /usr/local/bin/power-edge-client

# Move configs
sudo mv /tmp/power-edge-configs/*.yaml /etc/power-edge/
sudo chmod 644 /etc/power-edge/*.yaml

# Create service user (if not exists)
if ! id power-edge >/dev/null 2>&1; then
    sudo useradd --system --no-create-home --shell /bin/false power-edge
fi

# Create systemd service
sudo tee /etc/systemd/system/power-edge.service > /dev/null << 'SERVICE'
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
sudo systemctl daemon-reload

# Enable and start service
sudo systemctl enable power-edge
sudo systemctl start power-edge

# Wait for service to start
sleep 2

# Check status
if sudo systemctl is-active --quiet power-edge; then
    echo "✅ Service started successfully"
    sudo systemctl status power-edge --no-pager
else
    echo "❌ Service failed to start"
    sudo journalctl -u power-edge -n 50 --no-pager
    exit 1
fi
REMOTE_INSTALL

echo ""
echo "✅ Deployment complete!"
echo ""

# Test endpoints
echo "🧪 Testing endpoints..."
REMOTE_IP=$(echo "$SSH_HOST" | cut -d'@' -f2)

for endpoint in /version /health /metrics; do
    echo -n "  $endpoint: "
    if ssh "$SSH_HOST" "curl -sf http://localhost:9100$endpoint" > /dev/null 2>&1; then
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
echo "   ssh $SSH_HOST sudo journalctl -u power-edge -f"
echo ""
echo "🛑 Stop service:"
echo "   ssh $SSH_HOST sudo systemctl stop power-edge"
echo ""
