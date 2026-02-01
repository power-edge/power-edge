#!/usr/bin/env bash
set -euo pipefail

# Install power-edge on a remote node via SSH
# Usage: ./install-remote.sh <node-config-dir>
# Example: ./install-remote.sh data/nodes/stella-PowerEdge-T420
#
# Optional: Set SUDO_PASS environment variable for remote sudo password
# export SUDO_PASS=your-password

NODE_CONFIG_DIR="${1:-}"

if [ -z "$NODE_CONFIG_DIR" ]; then
    echo "Usage: $0 <node-config-dir>"
    echo "Example: $0 data/nodes/stella-PowerEdge-T420"
    echo ""
    echo "Optional: Set SUDO_PASS for remote sudo password"
    echo "  export SUDO_PASS=your-password"
    exit 1
fi

if [ ! -d "$NODE_CONFIG_DIR" ]; then
    echo "❌ Error: Node config directory not found: $NODE_CONFIG_DIR"
    exit 1
fi

# Read SSH connection info from connection.yaml
CONNECTION_FILE="$NODE_CONFIG_DIR/connection.yaml"
if [ ! -f "$CONNECTION_FILE" ]; then
    echo "❌ Error: Connection file not found: $CONNECTION_FILE"
    echo "   Create $CONNECTION_FILE with:"
    echo "   ssh:"
    echo "     user: \"username\""
    echo "     host: \"hostname_or_ip\""
    exit 1
fi

# Parse YAML (simple approach for our known structure)
SSH_USER=$(grep -A 2 "^ssh:" "$CONNECTION_FILE" | grep "user:" | sed 's/.*user: *"\(.*\)".*/\1/')
SSH_HOST=$(grep -A 2 "^ssh:" "$CONNECTION_FILE" | grep "host:" | sed 's/.*host: *"\(.*\)".*/\1/')
SSH_PORT=$(grep -A 3 "^ssh:" "$CONNECTION_FILE" | grep "port:" | sed 's/.*port: *\(.*\).*/\1/' || echo "22")

if [ -z "$SSH_USER" ] || [ -z "$SSH_HOST" ]; then
    echo "❌ Error: Could not parse SSH user/host from $CONNECTION_FILE"
    exit 1
fi

SSH_DEST="$SSH_USER@$SSH_HOST"

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

# Test connectivity
echo "🔌 Testing SSH connectivity..."
if ! ssh -o ConnectTimeout=5 "$SSH_DEST" "echo 'Connection OK'" >/dev/null 2>&1; then
    echo "❌ Cannot connect to $SSH_DEST"
    exit 1
fi
echo "✅ Connected"
echo ""

# Check if Go is installed, install/upgrade if needed
echo "🔍 Checking for Go installation on target..."
REQUIRED_GO_VERSION="1.23.5"

if ssh "$SSH_DEST" "command -v go >/dev/null 2>&1"; then
    INSTALLED_GO_VERSION=$(ssh "$SSH_DEST" "go version | awk '{print \$3}' | sed 's/go//'")
    echo "   Found Go $INSTALLED_GO_VERSION"

    # Check if we need to upgrade
    if [ "$INSTALLED_GO_VERSION" != "$REQUIRED_GO_VERSION" ]; then
        echo "   Upgrading Go to $REQUIRED_GO_VERSION..."
        NEED_INSTALL=true
    else
        echo "✅ Go $REQUIRED_GO_VERSION already installed"
        NEED_INSTALL=false
    fi
else
    echo "📥 Go not found, installing..."
    NEED_INSTALL=true
fi

if [ "$NEED_INSTALL" = true ]; then
    ssh "$SSH_DEST" "SUDO_PASS='${SUDO_PASS:-}'" 'bash -s' << 'GO_INSTALL'
set -euo pipefail

# Setup sudo command
if [ -n "${SUDO_PASS:-}" ]; then
    SUDO_PREFIX="echo \"$SUDO_PASS\" | sudo -S"
else
    SUDO_PREFIX="sudo"
fi

GO_VERSION="1.23.5"
GO_ARCH="amd64"

echo "   Downloading Go $GO_VERSION..."
wget -q -O /tmp/go.tar.gz https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz

echo "   Installing Go to /usr/local/go..."
eval $SUDO_PREFIX rm -rf /usr/local/go
eval $SUDO_PREFIX tar -C /usr/local -xzf /tmp/go.tar.gz
rm /tmp/go.tar.gz

echo "   Adding Go to PATH..."
if ! grep -q '/usr/local/go/bin' ~/.bashrc; then
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
fi
export PATH=$PATH:/usr/local/go/bin

echo "   ✅ Go $GO_VERSION installed"
GO_INSTALL

    echo "✅ Go $REQUIRED_GO_VERSION installed successfully"
fi

# Check if Git is installed (required for GitOps sync)
echo "🔍 Checking for Git installation..."
if ! ssh "$SSH_DEST" "command -v git >/dev/null 2>&1"; then
    echo "📥 Installing Git..."
    ssh "$SSH_DEST" "SUDO_PASS='${SUDO_PASS:-}'" 'bash -s' << 'GIT_INSTALL'
set -euo pipefail

# Setup sudo command
if [ -n "${SUDO_PASS:-}" ]; then
    SUDO_PREFIX="echo \"$SUDO_PASS\" | sudo -S"
else
    SUDO_PREFIX="sudo"
fi

echo "   Installing git..."
eval $SUDO_PREFIX apt-get update -qq
eval $SUDO_PREFIX apt-get install -y -qq git

echo "   ✅ Git installed"
GIT_INSTALL

    echo "✅ Git installed successfully"
else
    GIT_VERSION=$(ssh "$SSH_DEST" "git --version | awk '{print \$3}'")
    echo "✅ Git $GIT_VERSION already installed"
fi
echo ""

# Install systemd development headers (required for go-systemd)
echo "🔍 Checking for systemd development headers..."
if ! ssh "$SSH_DEST" "pkg-config --exists libsystemd 2>/dev/null"; then
    echo "📥 Installing libsystemd-dev..."
    ssh "$SSH_DEST" "SUDO_PASS='${SUDO_PASS:-}'" 'bash -s' << 'SYSTEMD_INSTALL'
set -euo pipefail

# Setup sudo command
if [ -n "${SUDO_PASS:-}" ]; then
    SUDO_PREFIX="echo \"$SUDO_PASS\" | sudo -S"
else
    SUDO_PREFIX="sudo"
fi

echo "   Installing systemd development packages..."
eval $SUDO_PREFIX apt-get update -qq
eval $SUDO_PREFIX apt-get install -y -qq libsystemd-dev pkg-config

echo "   ✅ systemd development headers installed"
SYSTEMD_INSTALL

    echo "✅ systemd development headers installed"
else
    echo "✅ systemd development headers already installed"
fi
echo ""

# Build on target (go-systemd cannot be cross-compiled from macOS)
echo "🔨 Building power-edge-client on target machine..."

# Create temp directory for source
TEMP_BUILD_DIR="/tmp/power-edge-build-$$"
ssh "$SSH_DEST" "mkdir -p $TEMP_BUILD_DIR"

# Copy source files (excluding .git, bin/, vendor/, etc.)
echo "📦 Copying source code to target..."
rsync -az --exclude='.git' --exclude='bin/' --exclude='vendor/' --exclude='.idea/' --exclude='.vscode/' --exclude='discovery/' ./ "$SSH_DEST:$TEMP_BUILD_DIR/"

# Build on target
echo "🔧 Building on target machine..."
ssh "$SSH_DEST" "export PATH=\$PATH:/usr/local/go/bin && cd $TEMP_BUILD_DIR && go build -ldflags \"-X main.Version=\$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') -X main.GitCommit=\$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -X main.BuildTime=\$(date -u '+%Y-%m-%d_%H:%M:%S')\" -o /tmp/power-edge-client ./cmd/power-edge-client"

if [ $? -eq 0 ]; then
    echo "✅ Build complete on target"
else
    echo "❌ Build failed on target"
    ssh "$SSH_DEST" "rm -rf $TEMP_BUILD_DIR"
    exit 1
fi

# Cleanup build directory
ssh "$SSH_DEST" "rm -rf $TEMP_BUILD_DIR"
echo ""

# Upload configs
echo "📝 Uploading configs..."
ssh "$SSH_DEST" "mkdir -p /tmp/power-edge-configs"
scp "$NODE_CONFIG_DIR"/*.yaml "$SSH_DEST:/tmp/power-edge-configs/"
echo "✅ Configs uploaded"

# Extract node name from config directory for GitOps path
NODE_NAME=$(basename "$NODE_CONFIG_DIR")
echo ""
echo "📝 Node name: $NODE_NAME"

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

ssh "$SSH_DEST" "SUDO_PASS='${SUDO_PASS:-}' NODE_NAME='$NODE_NAME'" 'bash -s' << 'REMOTE_INSTALL'
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
cat > /tmp/power-edge-sudoers << 'SUDOERS'
# Power Edge reconciliation commands
power-edge ALL=(ALL) NOPASSWD: /usr/bin/systemctl
power-edge ALL=(ALL) NOPASSWD: /usr/sbin/sysctl
power-edge ALL=(ALL) NOPASSWD: /usr/sbin/ufw
power-edge ALL=(ALL) NOPASSWD: /usr/bin/apt-get
power-edge ALL=(ALL) NOPASSWD: /usr/bin/yum
power-edge ALL=(ALL) NOPASSWD: /usr/bin/dnf
power-edge ALL=(ALL) NOPASSWD: /usr/sbin/iptables
SUDOERS

eval $SUDO_PREFIX cp /tmp/power-edge-sudoers /etc/sudoers.d/power-edge
eval $SUDO_PREFIX chmod 440 /etc/sudoers.d/power-edge
eval $SUDO_PREFIX chown root:root /etc/sudoers.d/power-edge
rm -f /tmp/power-edge-sudoers

# Create systemd template service with GitOps configuration
# Use environment variable for node name in GitOps path
cat > /tmp/power-edge@.service <<SERVICE
[Unit]
Description=Power Edge Client (%i)
After=network.target

[Service]
Type=simple
User=power-edge
ExecStart=/usr/local/bin/power-edge-client \\
  -state-config=/etc/power-edge/generated-state.yaml \\
  -watcher-config=/etc/power-edge/generated-watcher-config.yaml \\
  -listen=:9100 \\
  -reconcile=enforce \\
  -gitops-repo=https://github.com/power-edge/power-edge.git \\
  -gitops-branch=main \\
  -gitops-path=data/nodes/${NODE_NAME}/generated-state.yaml \\
  -gitops-interval=30s
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
