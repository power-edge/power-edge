#!/bin/bash
# About This Node - Comprehensive system identity report
# Like "About This Mac" but for Power Edge nodes

set -euo pipefail

SSH_HOST="${1:-}"
REMOTE=false

if [ -n "$SSH_HOST" ]; then
    REMOTE=true
    echo "🔍 About Node: $SSH_HOST"
else
    echo "🔍 About This Node"
fi

# Function to run command locally or remotely
run_cmd() {
    if $REMOTE; then
        ssh "$SSH_HOST" "$@" 2>/dev/null || echo "N/A"
    else
        eval "$@" 2>/dev/null || echo "N/A"
    fi
}

# Colors for output
BOLD='\033[1m'
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

section() {
    echo ""
    echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}${BLUE}$1${NC}"
    echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

item() {
    echo -e "  ${GREEN}$1:${NC} $2"
}

subsection() {
    echo -e "\n  ${YELLOW}▸ $1${NC}"
}

# ============================================================================
# NODE IDENTITY
# ============================================================================
section "📋 Node Identity"

HOSTNAME=$(run_cmd hostname)
FQDN=$(run_cmd "hostname -f")
DOMAIN=$(run_cmd "hostname -d")

item "Hostname" "$HOSTNAME"
item "FQDN" "$FQDN"
item "Domain" "$DOMAIN"

# Try to infer purpose from running services
SERVICES=$(run_cmd "systemctl list-units --type=service --state=running --no-pager --plain | awk '{print \$1}' | grep -E 'docker|openvpn|wireguard|k3s|kubelet|nginx|apache|postgres|mysql' || echo 'none'")

ROLES=()
if echo "$SERVICES" | grep -q "docker"; then ROLES+=("Container Host"); fi
if echo "$SERVICES" | grep -q "openvpn"; then ROLES+=("VPN Gateway (OpenVPN)"); fi
if echo "$SERVICES" | grep -q "wireguard"; then ROLES+=("VPN Gateway (WireGuard)"); fi
if echo "$SERVICES" | grep -q -E "k3s|kubelet"; then ROLES+=("Kubernetes Node"); fi
if echo "$SERVICES" | grep -q -E "nginx|apache"; then ROLES+=("Web Server"); fi
if echo "$SERVICES" | grep -q -E "postgres|mysql"; then ROLES+=("Database Server"); fi

if [ ${#ROLES[@]} -gt 0 ]; then
    item "Inferred Roles" "$(IFS=', '; echo "${ROLES[*]}")"
else
    item "Inferred Roles" "Generic Linux Server"
fi

# ============================================================================
# HARDWARE
# ============================================================================
section "🖥️  Hardware"

# System info
MANUFACTURER=$(run_cmd "sudo dmidecode -s system-manufacturer" | head -1)
PRODUCT=$(run_cmd "sudo dmidecode -s system-product-name" | head -1)
SERIAL=$(run_cmd "sudo dmidecode -s system-serial-number" | head -1)
UUID=$(run_cmd "sudo dmidecode -s system-uuid" | head -1)

item "Manufacturer" "$MANUFACTURER"
item "Model" "$PRODUCT"
item "Serial Number" "$SERIAL"
item "UUID" "$UUID"

# CPU
subsection "Processor"
CPU_MODEL=$(run_cmd "lscpu | grep 'Model name' | cut -d':' -f2 | xargs")
CPU_ARCH=$(run_cmd "uname -m")
CPU_CORES=$(run_cmd "nproc")
CPU_THREADS=$(run_cmd "lscpu | grep '^CPU(s):' | awk '{print \$2}'")
CPU_SOCKETS=$(run_cmd "lscpu | grep 'Socket(s):' | awk '{print \$2}'")

item "Model" "$CPU_MODEL"
item "Architecture" "$CPU_ARCH"
item "Physical Cores" "$CPU_CORES"
item "Logical CPUs" "$CPU_THREADS"
item "Sockets" "$CPU_SOCKETS"

# Memory
subsection "Memory"
MEM_TOTAL_KB=$(run_cmd "grep MemTotal /proc/meminfo | awk '{print \$2}'")
MEM_TOTAL_GB=$((MEM_TOTAL_KB / 1024 / 1024))
MEM_AVAILABLE_KB=$(run_cmd "grep MemAvailable /proc/meminfo | awk '{print \$2}'")
MEM_AVAILABLE_GB=$((MEM_AVAILABLE_KB / 1024 / 1024))
SWAP_TOTAL_KB=$(run_cmd "grep SwapTotal /proc/meminfo | awk '{print \$2}'")
SWAP_TOTAL_GB=$((SWAP_TOTAL_KB / 1024 / 1024))

item "Total" "${MEM_TOTAL_GB} GB"
item "Available" "${MEM_AVAILABLE_GB} GB"
item "Swap" "${SWAP_TOTAL_GB} GB"

# Storage
subsection "Storage"
run_cmd "df -h --output=source,size,used,avail,pcent,target -x tmpfs -x devtmpfs" | while IFS= read -r line; do
    echo "    $line"
done

# ============================================================================
# OPERATING SYSTEM
# ============================================================================
section "🐧 Operating System"

OS_NAME=$(run_cmd "cat /etc/os-release | grep '^PRETTY_NAME=' | cut -d'\"' -f2")
OS_ID=$(run_cmd "cat /etc/os-release | grep '^ID=' | cut -d'=' -f2")
OS_VERSION=$(run_cmd "cat /etc/os-release | grep '^VERSION_ID=' | cut -d'\"' -f2")
KERNEL=$(run_cmd "uname -r")
KERNEL_VERSION=$(run_cmd "uname -v")
UPTIME=$(run_cmd "uptime -p")
BOOT_TIME=$(run_cmd "uptime -s")

item "Distribution" "$OS_NAME"
item "OS ID" "$OS_ID"
item "Version" "$OS_VERSION"
item "Kernel" "$KERNEL"
item "Kernel Version" "$KERNEL_VERSION"
item "Uptime" "$UPTIME"
item "Boot Time" "$BOOT_TIME"

# ============================================================================
# NETWORK
# ============================================================================
section "🌐 Network"

# Interfaces
subsection "Network Interfaces"
run_cmd "ip -br addr show" | while IFS= read -r line; do
    echo "    $line"
done

# Routing
subsection "Default Route"
DEFAULT_ROUTE=$(run_cmd "ip route | grep default")
item "Gateway" "$DEFAULT_ROUTE"

# DNS
subsection "DNS"
NAMESERVERS=$(run_cmd "cat /etc/resolv.conf | grep '^nameserver' | awk '{print \$2}' | tr '\n' ', ' | sed 's/,$//'")
item "Nameservers" "$NAMESERVERS"

# VPN Detection
subsection "VPN Configuration"
VPN_DETECTED=false

# Check OpenVPN
if run_cmd "systemctl is-active openvpn@server" | grep -q "active"; then
    VPN_DETECTED=true
    OPENVPN_NETWORK=$(run_cmd "ip addr show tun0 2>/dev/null | grep 'inet ' | awk '{print \$2}'" || echo "N/A")
    item "OpenVPN Server" "Active"
    item "VPN Network (tun0)" "$OPENVPN_NETWORK"
fi

# Check WireGuard
if run_cmd "command -v wg" | grep -q "wg"; then
    WG_INTERFACES=$(run_cmd "sudo wg show interfaces" || echo "")
    if [ -n "$WG_INTERFACES" ]; then
        VPN_DETECTED=true
        item "WireGuard" "Active"
        item "Interfaces" "$WG_INTERFACES"
    fi
fi

# Check Tailscale
if run_cmd "command -v tailscale" | grep -q "tailscale"; then
    TS_STATUS=$(run_cmd "tailscale status --self --json 2>/dev/null | jq -r '.Self.TailscaleIPs[0]'" || echo "N/A")
    if [ "$TS_STATUS" != "N/A" ]; then
        VPN_DETECTED=true
        item "Tailscale" "Active"
        item "Tailscale IP" "$TS_STATUS"
    fi
fi

if ! $VPN_DETECTED; then
    item "Status" "No VPN detected"
fi

# ============================================================================
# NETWORK TUNING
# ============================================================================
section "🔧 Network Tuning"

IP_FORWARD=$(run_cmd "sysctl -n net.ipv4.ip_forward")
BRIDGE_NF=$(run_cmd "sysctl -n net.bridge.bridge-nf-call-iptables 2>/dev/null" || echo "N/A")
TCP_KEEPALIVE=$(run_cmd "sysctl -n net.ipv4.tcp_keepalive_time")
RMEM_MAX=$(run_cmd "sysctl -n net.core.rmem_max")
WMEM_MAX=$(run_cmd "sysctl -n net.core.wmem_max")

item "IP Forwarding" "$IP_FORWARD ($([ "$IP_FORWARD" = "1" ] && echo "Enabled" || echo "Disabled"))"
item "Bridge Netfilter" "$BRIDGE_NF"
item "TCP Keepalive" "${TCP_KEEPALIVE}s"
item "Max Receive Buffer" "$(numfmt --to=iec $RMEM_MAX 2>/dev/null || echo $RMEM_MAX)"
item "Max Send Buffer" "$(numfmt --to=iec $WMEM_MAX 2>/dev/null || echo $WMEM_MAX)"

# ============================================================================
# SYSTEM TUNING
# ============================================================================
section "⚙️  System Tuning"

SWAPPINESS=$(run_cmd "sysctl -n vm.swappiness")
INOTIFY_WATCHES=$(run_cmd "sysctl -n fs.inotify.max_user_watches")
KERNEL_PANIC=$(run_cmd "sysctl -n kernel.panic")
MAX_MAP_COUNT=$(run_cmd "sysctl -n vm.max_map_count")

item "Swappiness" "$SWAPPINESS"
item "Max Inotify Watches" "$INOTIFY_WATCHES"
item "Kernel Panic Timeout" "${KERNEL_PANIC}s"
item "Max Memory Maps" "$MAX_MAP_COUNT"

# ============================================================================
# CONTAINER RUNTIME
# ============================================================================
section "🐳 Container Runtime"

# Docker
if run_cmd "command -v docker" | grep -q "docker"; then
    DOCKER_VERSION=$(run_cmd "docker --version" | cut -d' ' -f3 | tr -d ',')
    DOCKER_RUNNING=$(run_cmd "systemctl is-active docker" || echo "inactive")
    CONTAINER_COUNT=$(run_cmd "docker ps -q | wc -l" || echo "0")
    IMAGE_COUNT=$(run_cmd "docker images -q | wc -l" || echo "0")

    subsection "Docker"
    item "Version" "$DOCKER_VERSION"
    item "Status" "$DOCKER_RUNNING"
    item "Running Containers" "$CONTAINER_COUNT"
    item "Images" "$IMAGE_COUNT"

    if [ "$CONTAINER_COUNT" != "0" ]; then
        echo ""
        echo "    Running Containers:"
        run_cmd "docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'" | tail -n +2 | while IFS= read -r line; do
            echo "      $line"
        done
    fi
fi

# Podman
if run_cmd "command -v podman" | grep -q "podman"; then
    PODMAN_VERSION=$(run_cmd "podman --version" | awk '{print $3}')
    subsection "Podman"
    item "Version" "$PODMAN_VERSION"
fi

# ============================================================================
# SERVICES
# ============================================================================
section "🔧 Critical Services"

subsection "System Services"
CRITICAL_SERVICES=("ssh" "docker" "openvpn@server" "wireguard" "k3s" "kubelet" "prometheus-node-exporter")

for service in "${CRITICAL_SERVICES[@]}"; do
    STATUS=$(run_cmd "systemctl is-active $service" || echo "not-found")
    if [ "$STATUS" != "not-found" ]; then
        ENABLED=$(run_cmd "systemctl is-enabled $service" || echo "unknown")
        STATUS_ICON="❌"
        [ "$STATUS" = "active" ] && STATUS_ICON="✅"

        echo -e "    $STATUS_ICON ${service}: ${STATUS} (${ENABLED})"
    fi
done

# ============================================================================
# SECURITY
# ============================================================================
section "🔒 Security & Access Control"

# Firewall
subsection "Firewall"
if run_cmd "command -v ufw" | grep -q "ufw"; then
    UFW_STATUS=$(run_cmd "sudo ufw status" | grep "Status:" | awk '{print $2}')
    UFW_RULES=$(run_cmd "sudo ufw status numbered | grep -c '^\[' || echo 0")
    item "UFW" "$UFW_STATUS ($UFW_RULES rules)"
elif run_cmd "command -v firewall-cmd" | grep -q "firewall-cmd"; then
    FIREWALLD_STATUS=$(run_cmd "systemctl is-active firewalld")
    item "firewalld" "$FIREWALLD_STATUS"
else
    item "Firewall" "None detected (UFW/firewalld)"
fi

# SSH
subsection "SSH Configuration"
SSH_PORT=$(run_cmd "grep -E '^Port ' /etc/ssh/sshd_config | awk '{print \$2}'" || echo "22")
SSH_PASS_AUTH=$(run_cmd "grep -E '^PasswordAuthentication ' /etc/ssh/sshd_config | awk '{print \$2}'" || echo "yes")
SSH_ROOT_LOGIN=$(run_cmd "grep -E '^PermitRootLogin ' /etc/ssh/sshd_config | awk '{print \$2}'" || echo "yes")

item "Port" "$SSH_PORT"
item "Password Auth" "$SSH_PASS_AUTH"
item "Root Login" "$SSH_ROOT_LOGIN"

# ============================================================================
# MONITORING
# ============================================================================
section "📊 Monitoring"

# Prometheus Node Exporter
if run_cmd "systemctl is-active prometheus-node-exporter" | grep -q "active"; then
    NODE_EXPORTER_VERSION=$(run_cmd "curl -s http://localhost:9100/metrics | grep 'node_exporter_build_info' | head -1" || echo "Running")
    item "Prometheus Node Exporter" "Active on :9100"
else
    item "Prometheus Node Exporter" "Not running"
fi

# Check if Power Edge is running
if run_cmd "systemctl is-active edge-state-exporter" | grep -q "active"; then
    item "Power Edge Exporter" "Active"
elif run_cmd "pgrep -f edge-state-exporter" >/dev/null 2>&1; then
    item "Power Edge Exporter" "Running (no systemd)"
else
    item "Power Edge Exporter" "Not installed"
fi

# ============================================================================
# SUMMARY
# ============================================================================
section "📝 Summary"

# Generate suggested tags
SUGGESTED_TAGS=()

# Role-based tags
[[ "$SERVICES" =~ docker ]] && SUGGESTED_TAGS+=("docker-host")
[[ "$SERVICES" =~ openvpn ]] && SUGGESTED_TAGS+=("vpn-gateway" "openvpn")
[[ "$SERVICES" =~ wireguard ]] && SUGGESTED_TAGS+=("vpn-gateway" "wireguard")
[[ "$SERVICES" =~ k3s|kubelet ]] && SUGGESTED_TAGS+=("kubernetes" "k8s-node")

# Hardware-based tags
[[ "$PRODUCT" =~ PowerEdge ]] && SUGGESTED_TAGS+=("dell-poweredge")
[[ $MEM_TOTAL_GB -gt 64 ]] && SUGGESTED_TAGS+=("high-memory")
[[ $CPU_CORES -gt 16 ]] && SUGGESTED_TAGS+=("high-cpu")

# Environment tags
SUGGESTED_TAGS+=("$(echo $HOSTNAME | grep -oE '(prod|dev|test|staging|lab)' || echo 'unknown-env')")

echo ""
item "Suggested Tags" "$(IFS=', '; echo "${SUGGESTED_TAGS[*]}")"
echo ""
item "Configuration Path" "data/nodes/$HOSTNAME/"
echo ""

echo -e "${GREEN}✅ System identity report complete${NC}"
echo ""
