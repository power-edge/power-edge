#!/usr/bin/env bash
set -euo pipefail

# Organize discovered config into node-specific directory
# Usage: ./organize-config.sh [discovered-config-dir]

DISCOVERED_DIR="${1:-./discovered-config}"

if [ ! -d "$DISCOVERED_DIR" ]; then
    echo "❌ Error: Directory not found: $DISCOVERED_DIR"
    echo "Usage: $0 [discovered-config-dir]"
    exit 1
fi

# Extract hostname from system-info.json
if [ -f "$DISCOVERED_DIR/system-info.json" ]; then
    HOSTNAME=$(jq -r '.hostname' "$DISCOVERED_DIR/system-info.json")
else
    echo "⚠️  Warning: system-info.json not found, using current hostname"
    HOSTNAME=$(hostname -s)
fi

if [ -z "$HOSTNAME" ] || [ "$HOSTNAME" = "null" ]; then
    echo "❌ Error: Could not determine hostname"
    exit 1
fi

NODE_DIR="config/nodes/$HOSTNAME"

echo "📦 Organizing config for node: $HOSTNAME"
echo ""

# Create node directory
mkdir -p "$NODE_DIR"
echo "✓ Created directory: $NODE_DIR"

# Copy generated configs
if ls "$DISCOVERED_DIR"/generated-*.yaml 1> /dev/null 2>&1; then
    cp "$DISCOVERED_DIR"/generated-*.yaml "$NODE_DIR/"
    echo "✓ Copied generated configs:"
    ls -lh "$NODE_DIR"/generated-*.yaml | awk '{print "  -", $9, "(" $5 ")"}'
else
    echo "⚠️  No generated-*.yaml files found in $DISCOVERED_DIR"
fi

# Copy system identity if exists
if [ -f "$DISCOVERED_DIR/system-identity.yaml" ]; then
    cp "$DISCOVERED_DIR/system-identity.yaml" "$NODE_DIR/"
    echo "✓ Copied system-identity.yaml"
fi

# Copy raw discovery data for reference
mkdir -p "$NODE_DIR/discovery-data"
cp "$DISCOVERED_DIR"/*.{txt,json} "$NODE_DIR/discovery-data/" 2>/dev/null || true
echo "✓ Copied raw discovery data to $NODE_DIR/discovery-data/"

echo ""
echo "✅ Config organized successfully!"
echo ""
echo "Next steps:"
echo "  1. Review configs:    ls -la $NODE_DIR/"
echo "  2. Build:             make build"
echo "  3. Test locally:      make run"
echo "  4. Deploy:            make install"
echo ""
echo "Node directory structure:"
tree -L 2 "$NODE_DIR" 2>/dev/null || find "$NODE_DIR" -type f
