#!/bin/bash
# Collect system identity information
# Generates system-identity.yaml with immutable identifiers

set -euo pipefail

SSH_HOST="${1:-}"
REMOTE=false

if [ -n "$SSH_HOST" ]; then
    REMOTE=true
fi

# Function to run command locally or remotely
run_cmd() {
    if $REMOTE; then
        ssh "$SSH_HOST" "$@" 2>/dev/null || echo "N/A"
    else
        eval "$@" 2>/dev/null || echo "N/A"
    fi
}

echo "🔍 Collecting system identity..."

# Detect platform
PLATFORM=$(run_cmd "uname -s")

case "$PLATFORM" in
    Linux)
        OS_TYPE="linux"
        echo "  Platform: Linux"

        # Primary ID: machine-id (systemd)
        MACHINE_ID=$(run_cmd "cat /etc/machine-id" | tr -d '\n')
        PRIMARY_ID="$MACHINE_ID"
        ID_TYPE="machine-id"

        # Hardware UUID (DMI/SMBIOS)
        DMI_UUID=$(run_cmd "sudo cat /sys/class/dmi/id/product_uuid" | tr '[:lower:]' '[:upper:]')
        DMI_BOARD_SERIAL=$(run_cmd "sudo cat /sys/class/dmi/id/board_serial")

        # OS info
        OS_FAMILY=$(run_cmd "cat /etc/os-release | grep '^ID_LIKE=' | cut -d'=' -f2 | tr -d '\"' | awk '{print \$1}'")
        [ -z "$OS_FAMILY" ] && OS_FAMILY=$(run_cmd "cat /etc/os-release | grep '^ID=' | cut -d'=' -f2 | tr -d '\"'")
        OS_VERSION=$(run_cmd "cat /etc/os-release | grep '^VERSION_ID=' | cut -d'\"' -f2")
        KERNEL_VERSION=$(run_cmd "uname -r")

        echo "  Machine ID: ${MACHINE_ID:0:8}...${MACHINE_ID: -8}"
        echo "  Hardware UUID: $DMI_UUID"
        ;;

    Darwin)
        OS_TYPE="darwin"
        echo "  Platform: macOS"

        # Primary ID: IOPlatformUUID
        IO_UUID=$(run_cmd "ioreg -d2 -c IOPlatformExpertDevice | awk -F\\\" '/IOPlatformUUID/{print \$(NF-1)}'")
        PRIMARY_ID="$IO_UUID"
        ID_TYPE="io-platform-uuid"

        # OS info
        OS_FAMILY="macos"
        OS_VERSION=$(run_cmd "sw_vers -productVersion")
        KERNEL_VERSION=$(run_cmd "uname -r")

        echo "  IOPlatformUUID: $IO_UUID"
        ;;

    *)
        echo "❌ Unsupported platform: $PLATFORM"
        exit 1
        ;;
esac

# Generate composite key: os_type:primary_id
COMPOSITE_KEY="${OS_TYPE}:${PRIMARY_ID}"

# Generate SHA256 hash of composite key
COMPOSITE_HASH=$(echo -n "$COMPOSITE_KEY" | shasum -a 256 | awk '{print $1}')

TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
COLLECTOR_VERSION="probe-v1.0.0"

echo "  Composite Key: $COMPOSITE_KEY"
echo "  Hash: ${COMPOSITE_HASH:0:16}..."

# Generate YAML output
cat > system-identity.yaml << EOF
# System Identity - Immutable node identification
# Generated: $TIMESTAMP

platform:
  os_type: $OS_TYPE
  os_family: $OS_FAMILY
  os_version: "$OS_VERSION"
  kernel_version: "$KERNEL_VERSION"

identifiers:
  primary_id: "$PRIMARY_ID"
  id_type: $ID_TYPE
EOF

# Add platform-specific identifiers
if [ "$OS_TYPE" = "linux" ]; then
    cat >> system-identity.yaml << EOF
  machine_id: "$MACHINE_ID"
EOF
    if [ "$DMI_UUID" != "N/A" ]; then
        cat >> system-identity.yaml << EOF
  dmi_product_uuid: "$DMI_UUID"
EOF
    fi
    if [ "$DMI_BOARD_SERIAL" != "N/A" ]; then
        cat >> system-identity.yaml << EOF
  dmi_board_serial: "$DMI_BOARD_SERIAL"
EOF
    fi
elif [ "$OS_TYPE" = "darwin" ]; then
    cat >> system-identity.yaml << EOF
  io_platform_uuid: "$IO_UUID"
  hardware_uuid: "$IO_UUID"
EOF
fi

# Add metadata
cat >> system-identity.yaml << EOF
  collected_at: "$TIMESTAMP"
  collected_by: "$COLLECTOR_VERSION"

composite_key:
  key: "$COMPOSITE_KEY"
  components:
    - name: os_type
      value: "$OS_TYPE"
    - name: primary_id
      value: "$PRIMARY_ID"
  hash: "$COMPOSITE_HASH"

validation:
  require_match: true
  check_on_startup: true
  allow_os_migration: false
  alert_on_mismatch: true

registration:
  registered: false
  registered_at: ""
  controller_url: ""
  registration_token: ""
  last_checkin: ""
EOF

echo ""
echo "✅ System identity collected"
echo ""
echo "📄 Output: system-identity.yaml"
echo ""
echo "Identity Summary:"
echo "  Composite Key:  $COMPOSITE_KEY"
echo "  Hash (SHA256):  $COMPOSITE_HASH"
echo "  Platform:       $OS_TYPE ($OS_FAMILY)"
echo "  OS Version:     $OS_VERSION"
echo ""
echo "Next steps:"
echo "  1. Review system-identity.yaml"
echo "  2. Store hash in database for registration"
echo "  3. Add to node config directory"
echo ""
