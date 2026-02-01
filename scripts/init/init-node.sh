#!/bin/bash
# Discover current state of edge node and generate config

set -euo pipefail

# Configuration
SSH_HOST="${1:-}"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

if [ -z "$SSH_HOST" ]; then
    echo "Usage: $0 <ssh-host>"
    echo "Example: $0 dell-t420.local"
    exit 1
fi

# Test connectivity first to get hostname
echo "🔍 Discovering edge node: $SSH_HOST"
echo "Testing SSH connectivity..."
if ! ssh -o ConnectTimeout=5 "$SSH_HOST" "echo 'Connection OK'" >/dev/null 2>&1; then
    echo "❌ Cannot connect to $SSH_HOST"
    echo "   Make sure:"
    echo "   1. The host is reachable"
    echo "   2. SSH is configured"
    echo "   3. You have passwordless access"
    exit 1
fi

# Get hostname for output directory
REMOTE_HOSTNAME=$(ssh "$SSH_HOST" "hostname -s" 2>/dev/null || echo "unknown")
OUTPUT_DIR="/tmp/power-edge-init-${REMOTE_HOSTNAME}-${TIMESTAMP}"

echo "✅ Connected to $SSH_HOST (hostname: $REMOTE_HOSTNAME)"
echo "📁 Output directory: $OUTPUT_DIR"
echo ""

mkdir -p "$OUTPUT_DIR"

# Function to run command on remote host
remote_exec() {
    ssh "$SSH_HOST" "$@" 2>/dev/null || echo "COMMAND_FAILED"
}

# ========================================
# 1. System Information
# ========================================
echo "📊 Gathering system information..."
HOSTNAME=$(remote_exec hostname)
KERNEL=$(remote_exec "uname -r")
OS=$(remote_exec "cat /etc/os-release | grep PRETTY_NAME | cut -d'\"' -f2")
ARCH=$(remote_exec "uname -m")
CPU_CORES=$(remote_exec nproc)
MEMORY_GB=$(remote_exec "free -g | awk '/^Mem:/{print \$2}'")

cat > "$OUTPUT_DIR/system-info.json" << EOF
{
  "hostname": "$HOSTNAME",
  "kernel": "$KERNEL",
  "os": "$OS",
  "architecture": "$ARCH",
  "cpu_cores": $CPU_CORES,
  "memory_gb": $MEMORY_GB,
  "discovered_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

echo "  Hostname: $HOSTNAME"
echo "  OS: $OS"
echo "  CPU cores: $CPU_CORES"
echo "  Memory: ${MEMORY_GB}GB"
echo ""

# ========================================
# 2. Discover Running Services
# ========================================
echo "🔧 Discovering systemd services..."
remote_exec "systemctl list-units --type=service --state=running --no-pager --plain" | \
  awk '{print $1}' | grep '.service$' | sort > "$OUTPUT_DIR/running-services.txt"

# Count services
SERVICE_COUNT=$(wc -l < "$OUTPUT_DIR/running-services.txt")
echo "  Found $SERVICE_COUNT running services"

# Extract critical services
critical_services=$(cat "$OUTPUT_DIR/running-services.txt" | \
  grep -E 'docker|kube|ssh|ufw|firewalld|prometheus|node_exporter|vpn|wireguard|tailscale' || true)

if [ -n "$critical_services" ]; then
    echo "  Critical services found:"
    echo "$critical_services" | sed 's/^/    - /'
    echo "$critical_services" > "$OUTPUT_DIR/critical-services.txt"
else
    echo "  No critical services detected"
    touch "$OUTPUT_DIR/critical-services.txt"
fi
echo ""

# ========================================
# 3. Discover Firewall Rules
# ========================================
echo "🛡️  Discovering firewall rules..."
echo "  Note: Requires passwordless sudo for complete firewall discovery"

if remote_exec "command -v ufw >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
    echo "  Found UFW"
    UFW_STATUS=$(remote_exec "sudo -n ufw status numbered 2>/dev/null")

    if [ "$UFW_STATUS" != "COMMAND_FAILED" ] && [ -n "$UFW_STATUS" ]; then
        echo "$UFW_STATUS" > "$OUTPUT_DIR/ufw-rules.txt"

        if echo "$UFW_STATUS" | grep -q "Status: active"; then
            RULE_COUNT=$(echo "$UFW_STATUS" | grep -c '^\[' || echo "0")
            echo "  ✓ UFW is active with $RULE_COUNT rules"
        else
            echo "  ✓ UFW is installed but inactive"
        fi
    else
        echo "  ⚠️  UFW found but requires sudo password (skipping rules)"
        echo "UFW detected but requires sudo password" > "$OUTPUT_DIR/ufw-rules.txt"
    fi
elif remote_exec "command -v firewall-cmd >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
    echo "  Found firewalld"
    FIREWALLD_STATUS=$(remote_exec "sudo -n firewall-cmd --list-all 2>/dev/null")

    if [ "$FIREWALLD_STATUS" != "COMMAND_FAILED" ] && [ -n "$FIREWALLD_STATUS" ]; then
        echo "$FIREWALLD_STATUS" > "$OUTPUT_DIR/firewalld-rules.txt"
        echo "  ✓ Firewalld rules captured"
    else
        echo "  ⚠️  Firewalld found but requires sudo password (skipping rules)"
        echo "Firewalld detected but requires sudo password" > "$OUTPUT_DIR/firewalld-rules.txt"
    fi
else
    echo "  No UFW or firewalld detected, checking iptables..."
    IPTABLES_OUTPUT=$(remote_exec "sudo -n iptables -L -n -v 2>/dev/null")

    if [ "$IPTABLES_OUTPUT" != "COMMAND_FAILED" ] && [ -n "$IPTABLES_OUTPUT" ]; then
        echo "$IPTABLES_OUTPUT" > "$OUTPUT_DIR/iptables-rules.txt"
        RULE_COUNT=$(echo "$IPTABLES_OUTPUT" | grep -c '^Chain' || echo "0")
        echo "  ✓ iptables rules captured ($RULE_COUNT chains)"
    else
        echo "  ⚠️  iptables requires sudo password (skipping)"
        echo "No firewall rules captured - requires passwordless sudo" > "$OUTPUT_DIR/iptables-rules.txt"
        echo ""
        echo "  💡 To capture firewall rules, configure passwordless sudo on target:"
        echo "     echo '$USER ALL=(ALL) NOPASSWD: /usr/sbin/ufw, /usr/sbin/iptables' | sudo tee /etc/sudoers.d/power-edge-init"
    fi
fi
echo ""

# ========================================
# 4. Discover sysctl Parameters
# ========================================
echo "⚙️  Discovering sysctl parameters..."
important_sysctls=(
    "net.ipv4.ip_forward"
    "net.bridge.bridge-nf-call-iptables"
    "vm.swappiness"
    "fs.inotify.max_user_watches"
    "kernel.panic"
    "net.ipv4.tcp_keepalive_time"
)

echo "# Important sysctl parameters" > "$OUTPUT_DIR/sysctl-important.txt"
for param in "${important_sysctls[@]}"; do
    value=$(remote_exec "sysctl -n $param 2>/dev/null")
    if [ "$value" != "COMMAND_FAILED" ] && [ -n "$value" ]; then
        echo "$param = $value" >> "$OUTPUT_DIR/sysctl-important.txt"
        echo "  $param = $value"
    fi
done
echo ""

# ========================================
# 5. Discover Installed Packages
# ========================================
echo "📦 Discovering installed packages..."
if remote_exec "command -v dpkg >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
    PKG_COUNT=$(remote_exec "dpkg -l | grep '^ii' | wc -l")
    echo "  Found $PKG_COUNT packages (Debian/Ubuntu)"
    remote_exec "dpkg -l | grep '^ii' | awk '{print \$2}' | sort" > "$OUTPUT_DIR/installed-packages.txt"
elif remote_exec "command -v rpm >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
    PKG_COUNT=$(remote_exec "rpm -qa | wc -l")
    echo "  Found $PKG_COUNT packages (RHEL/CentOS)"
    remote_exec "rpm -qa | sort" > "$OUTPUT_DIR/installed-packages.txt"
fi

# Check for critical packages
critical_packages=(curl wget git docker-ce containerd prometheus-node-exporter)
echo "  Checking critical packages:"
for pkg in "${critical_packages[@]}"; do
    if grep -q "^$pkg" "$OUTPUT_DIR/installed-packages.txt" 2>/dev/null; then
        echo "    ✓ $pkg"
    fi
done
echo ""

# ========================================
# 6. Discover File Paths to Monitor
# ========================================
echo "📁 Discovering critical file paths..."
critical_paths=(
    "/etc/ssh/sshd_config"
    "/etc/ufw/user.rules"
    "/etc/ufw/user6.rules"
    "/etc/sysctl.conf"
    "/etc/sysctl.d"
    "/etc/systemd/system"
    "/etc/docker/daemon.json"
    "/etc/kubernetes"
    "/etc/hosts"
    "/etc/resolv.conf"
)

> "$OUTPUT_DIR/existing-paths.txt"
for path in "${critical_paths[@]}"; do
    if remote_exec "[ -e '$path' ]" | grep -v COMMAND_FAILED >/dev/null; then
        echo "$path" >> "$OUTPUT_DIR/existing-paths.txt"
        echo "  ✓ $path"
    fi
done
echo ""

# ========================================
# 7. Discover Container Runtime
# ========================================
echo "🐳 Discovering container runtime..."
if remote_exec "command -v docker >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
    DOCKER_VERSION=$(remote_exec "docker --version")
    echo "  $DOCKER_VERSION"

    CONTAINER_COUNT=$(remote_exec "docker ps -q | wc -l")
    echo "  Running containers: $CONTAINER_COUNT"

    if [ "$CONTAINER_COUNT" -gt 0 ]; then
        remote_exec "docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'" > "$OUTPUT_DIR/docker-containers.txt"
    fi
fi

if remote_exec "command -v kubectl >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
    KUBECTL_VERSION=$(remote_exec "kubectl version --client --short 2>/dev/null || kubectl version --client 2>/dev/null | head -1")
    echo "  $KUBECTL_VERSION"

    # Check if can access cluster
    if remote_exec "kubectl cluster-info >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
        NODE_COUNT=$(remote_exec "kubectl get nodes --no-headers | wc -l")
        echo "  Cluster accessible: $NODE_COUNT nodes"
        remote_exec "kubectl get nodes -o wide" > "$OUTPUT_DIR/k8s-nodes.txt" 2>/dev/null || true
        remote_exec "kubectl get pods -A --no-headers | wc -l" > "$OUTPUT_DIR/k8s-pod-count.txt" 2>/dev/null || true
    fi
fi
echo ""

# ========================================
# 8. Discover Network Configuration
# ========================================
echo "🌐 Discovering network configuration..."
remote_exec "ip addr show" > "$OUTPUT_DIR/network-interfaces.txt"
INTERFACE_COUNT=$(remote_exec "ip link show | grep '^[0-9]' | wc -l")
echo "  Network interfaces: $INTERFACE_COUNT"

# Check for VPN
if remote_exec "command -v wg >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
    echo "  ✓ WireGuard detected"
    remote_exec "sudo wg show" > "$OUTPUT_DIR/wireguard.txt" 2>/dev/null || true
fi

if remote_exec "command -v tailscale >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
    echo "  ✓ Tailscale detected"
    TS_STATUS=$(remote_exec "tailscale status --self")
    echo "$TS_STATUS" > "$OUTPUT_DIR/tailscale.txt"
    TS_IP=$(echo "$TS_STATUS" | awk '{print $1}')
    echo "    Tailscale IP: $TS_IP"
fi
echo ""

# ========================================
# 9. Discover Monitoring Tools
# ========================================
echo "📊 Discovering monitoring tools..."
monitoring_tools=(
    "prometheus"
    "node_exporter"
    "promtail"
    "grafana-server"
    "loki"
)

> "$OUTPUT_DIR/monitoring-tools.txt"
for tool in "${monitoring_tools[@]}"; do
    if remote_exec "command -v $tool >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
        version=$(remote_exec "$tool --version 2>&1 | head -1")
        echo "$tool: $version" >> "$OUTPUT_DIR/monitoring-tools.txt"
        echo "  ✓ $tool"
    fi
done
echo ""

# ========================================
# 10. Discover System Versions
# ========================================
echo "🔍 Discovering system component versions..."

# Initialize JSON with timestamp
DISCOVERED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo '{}' | jq --arg timestamp "$DISCOVERED_AT" '{discovered_at: $timestamp, versions: {}}' > "$OUTPUT_DIR/system-versions.json"

# Helper to add version to JSON
add_version() {
    local name="$1"
    local version="$2"

    if [ -n "$version" ] && [ "$version" != "COMMAND_FAILED" ]; then
        jq --arg name "$name" --arg version "$version" \
           '.versions[$name] = $version' \
           "$OUTPUT_DIR/system-versions.json" > "$OUTPUT_DIR/system-versions.json.tmp" && \
        mv "$OUTPUT_DIR/system-versions.json.tmp" "$OUTPUT_DIR/system-versions.json"
        echo "  $name: $version"
    fi
}

# Collect and add versions
KERNEL_VERSION=$(remote_exec "uname -r" | grep -v COMMAND_FAILED)
add_version "kernel" "$KERNEL_VERSION"

SYSTEMD_VERSION=$(remote_exec "systemctl --version | head -1 | awk '{print \$2}'" | grep -v COMMAND_FAILED)
add_version "systemd" "$SYSTEMD_VERSION"

UFW_VERSION=$(remote_exec "ufw version 2>/dev/null | awk 'NR==1{print \$2}'" | grep -v COMMAND_FAILED)
add_version "ufw" "$UFW_VERSION"

IPTABLES_VERSION=$(remote_exec "iptables --version 2>/dev/null | awk '{print \$2}' | sed 's/v//'" | grep -v COMMAND_FAILED)
add_version "iptables" "$IPTABLES_VERSION"

DOCKER_VERSION=$(remote_exec "docker --version 2>/dev/null | awk '{print \$3}' | sed 's/,//'" | grep -v COMMAND_FAILED)
add_version "docker" "$DOCKER_VERSION"

KUBECTL_VERSION=$(remote_exec "kubectl version --client --short 2>/dev/null | awk '{print \$3}' | sed 's/v//'" | grep -v COMMAND_FAILED)
add_version "kubectl" "$KUBECTL_VERSION"

GIT_VERSION=$(remote_exec "git --version 2>/dev/null | awk '{print \$3}'" | grep -v COMMAND_FAILED)
add_version "git" "$GIT_VERSION"

echo ""

# ========================================
# 11. Generate State Config
# ========================================
echo "📝 Generating state configuration..."

cat > "$OUTPUT_DIR/state.yaml" << EOF
version: "1.0"
metadata:
  site: "$HOSTNAME"
  environment: "home-lab"
  description: "Dell T420 Home Server - Auto-discovered $(date -u +%Y-%m-%dT%H:%M:%SZ)"

EOF

# Add firewall section
if [ -f "$OUTPUT_DIR/ufw-rules.txt" ]; then
    echo "firewall:" >> "$OUTPUT_DIR/state.yaml"
    echo "  enabled: true" >> "$OUTPUT_DIR/state.yaml"
    echo "  default_incoming: deny" >> "$OUTPUT_DIR/state.yaml"
    echo "  default_outgoing: allow" >> "$OUTPUT_DIR/state.yaml"
    echo "  rules:" >> "$OUTPUT_DIR/state.yaml"

    # Parse UFW rules (extract port/proto from numbered output)
    grep '^\[' "$OUTPUT_DIR/ufw-rules.txt" | grep 'ALLOW' | while IFS= read -r line; do
        # Try to extract port number
        if echo "$line" | grep -q '[0-9]\+/tcp'; then
            port=$(echo "$line" | grep -oE '[0-9]+/tcp' | head -1 | cut -d'/' -f1)
            proto="tcp"
        elif echo "$line" | grep -q '[0-9]\+/udp'; then
            port=$(echo "$line" | grep -oE '[0-9]+/udp' | head -1 | cut -d'/' -f1)
            proto="udp"
        else
            continue
        fi

        echo "    - port: $port" >> "$OUTPUT_DIR/state.yaml"
        echo "      proto: $proto" >> "$OUTPUT_DIR/state.yaml"
        echo "      action: allow" >> "$OUTPUT_DIR/state.yaml"
        echo "      from: any" >> "$OUTPUT_DIR/state.yaml"
        echo "      comment: \"Auto-discovered\"" >> "$OUTPUT_DIR/state.yaml"
    done
fi

# Add services section
if [ -f "$OUTPUT_DIR/critical-services.txt" ] && [ -s "$OUTPUT_DIR/critical-services.txt" ]; then
    echo "" >> "$OUTPUT_DIR/state.yaml"
    echo "services:" >> "$OUTPUT_DIR/state.yaml"
    while IFS= read -r service; do
        service_name=$(echo "$service" | sed 's/.service$//')
        echo "  - name: $service_name" >> "$OUTPUT_DIR/state.yaml"
        echo "    state: running" >> "$OUTPUT_DIR/state.yaml"
        echo "    enabled: true" >> "$OUTPUT_DIR/state.yaml"
    done < "$OUTPUT_DIR/critical-services.txt"
fi

# Add sysctl section
if [ -f "$OUTPUT_DIR/sysctl-important.txt" ]; then
    echo "" >> "$OUTPUT_DIR/state.yaml"
    echo "sysctl:" >> "$OUTPUT_DIR/state.yaml"
    while IFS= read -r line; do
        if [[ $line =~ ^([^=]+)\ =\ (.+)$ ]]; then
            key="${BASH_REMATCH[1]}"
            value="${BASH_REMATCH[2]}"
            # Skip comment lines
            if [[ ! $line =~ ^# ]]; then
                echo "  $key: \"$value\"" >> "$OUTPUT_DIR/state.yaml"
            fi
        fi
    done < "$OUTPUT_DIR/sysctl-important.txt"
fi

# ========================================
# 12. Generate Watcher Config
# ========================================
echo "📝 Generating watcher configuration..."

cat > "$OUTPUT_DIR/watcher-config.yaml" << EOF
# Auto-generated watcher configuration
version: "1.0"

watchers:
  enabled: true

  inotify:
    enabled: true
    paths:
EOF

# Add discovered paths
if [ -f "$OUTPUT_DIR/existing-paths.txt" ]; then
    while IFS= read -r path; do
        echo "      - $path" >> "$OUTPUT_DIR/watcher-config.yaml"
    done < "$OUTPUT_DIR/existing-paths.txt"
fi

# Add services to watch
if [ -f "$OUTPUT_DIR/critical-services.txt" ] && [ -s "$OUTPUT_DIR/critical-services.txt" ]; then
    cat >> "$OUTPUT_DIR/watcher-config.yaml" << EOF

  journald:
    enabled: true
    units:
EOF

    while IFS= read -r service; do
        echo "      - $service" >> "$OUTPUT_DIR/watcher-config.yaml"
    done < "$OUTPUT_DIR/critical-services.txt"
fi

# Add commands to audit
cat >> "$OUTPUT_DIR/watcher-config.yaml" << EOF

  auditd:
    enabled: true
    commands:
      - ufw
      - iptables
      - systemctl
      - sysctl
EOF

# Add docker/kubectl if found
if remote_exec "command -v docker >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
    echo "      - docker" >> "$OUTPUT_DIR/watcher-config.yaml"
fi
if remote_exec "command -v kubectl >/dev/null 2>&1" | grep -v COMMAND_FAILED >/dev/null; then
    echo "      - kubectl" >> "$OUTPUT_DIR/watcher-config.yaml"
fi

echo ""
echo "✅ Node initialization complete!"
echo ""
echo "📁 Results in: $OUTPUT_DIR/"
echo ""
echo "Generated files:"
ls -lh "$OUTPUT_DIR" | tail -n +2 | awk '{printf "  %s  %s\n", $9, $5}'
echo ""
echo "📋 Summary:"
cat "$OUTPUT_DIR/system-info.json" | grep -v '{' | grep -v '}' | sed 's/^/  /'
echo ""
echo "Next steps:"
echo "  1. Review configs:     ls -la $OUTPUT_DIR/"
echo "  2. Organize:           bash scripts/init/organize-config.sh $OUTPUT_DIR"
echo "  3. Build and test:     make build && make run"
echo ""
echo "Or manually organize:"
echo "  mkdir -p data/nodes/$HOSTNAME"
echo "  cp $OUTPUT_DIR/generated-*.yaml data/nodes/$HOSTNAME/"
echo ""
