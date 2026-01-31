# Deployment Quick Start

## Prerequisites

1. **Passwordless SSH** to your edge nodes
2. **One of**:
   - Passwordless sudo on edge node, OR
   - Remote sudo password stored in `.env`

## Setup

### 1. Create `.env` file

```bash
cp .env.example .env
```

Edit `.env`:
```bash
# SSH destination
export SSH=stella@10.8.0.1

# Optional: Remote sudo password (if you don't have passwordless sudo)
export SUDO_PASS=your-sudo-password-here

# Optional: Override auto-detected node config path
# export NODE_CONFIG=config/nodes/stella-PowerEdge-T420
```

### 2. Load environment

```bash
# Source .env in your current shell
source .env

# Or export manually
export SSH=stella@10.8.0.1
export SUDO_PASS=your-sudo-password  # optional
```

## Deployment Workflow

### Step 1: Initialize Node Configuration

```bash
# Using environment variable
make init

# Or inline
make init SSH=stella@10.8.0.1
```

This connects to your edge node and discovers:
- Running services
- Sysctl parameters
- Firewall rules
- System information

Output: `/tmp/power-edge-init-<hostname>-<timestamp>/`

### Step 2: Organize Configuration

```bash
# Auto-organize by hostname
bash scripts/init/organize-config.sh /tmp/power-edge-init-stella-PowerEdge-T420-*/
```

This creates: `config/nodes/<hostname>/` with:
- `generated-state.yaml` - Desired system state
- `generated-watcher-config.yaml` - Monitoring config
- `discovery-data/` - Raw discovery output

### Step 3: Review & Adjust Configuration

```bash
# Review what was discovered
cat config/nodes/stella-PowerEdge-T420/generated-state.yaml

# Edit if needed (add/remove services, adjust sysctl, etc.)
vim config/nodes/stella-PowerEdge-T420/generated-state.yaml
```

### Step 4: Deploy to Edge Node

```bash
# Using environment variables (.env or exported)
make deploy NODE_CONFIG=config/nodes/stella-PowerEdge-T420

# Or fully inline
make deploy SSH=stella@10.8.0.1 NODE_CONFIG=config/nodes/stella-PowerEdge-T420
```

This:
1. **Auto-detects target platform** (reads from discovery data or probes via SSH)
2. **Cross-compiles binary** for target platform (e.g., macOS → Linux)
3. Uploads binary and configs via SSH
4. Installs to `/usr/local/bin/power-edge-client`
5. Creates systemd service
6. Starts and enables service
7. Tests endpoints

**Cross-Platform Support**: The deploy script automatically detects the target node's OS and architecture (linux/darwin, amd64/arm64) and builds the correct binary. You can develop on macOS and deploy to Linux seamlessly!

### Step 5: Verify Deployment

```bash
# Check metrics endpoint
curl http://10.8.0.1:9100/metrics

# Check health
curl http://10.8.0.1:9100/health

# Check version
curl http://10.8.0.1:9100/version

# View logs on edge node
ssh "${SSH}" sudo journalctl -u power-edge -f
```

## Troubleshooting

### SSH Connection Failed
```bash
# Test SSH connectivity
ssh "${SSH}" echo "OK"

# Check SSH config
cat ~/.ssh/config
```

### Sudo Password Issues

**Option A: Configure Passwordless Sudo (Recommended)**

On edge node:
```bash
# Create sudoers rule for specific user
sudo visudo -f /etc/sudoers.d/power-edge

# Add (replace 'stella' with your username):
stella ALL=(ALL) NOPASSWD: ALL
```

**Option B: Use SUDO_PASS Environment Variable**

```bash
# In .env file
export SUDO_PASS=your-sudo-password

# Source and deploy
source .env
make deploy NODE_CONFIG=config/nodes/stella-PowerEdge-T420
```

### Service Won't Start

```bash
# Check service status
ssh "${SSH}" sudo systemctl status power-edge

# View full logs
ssh "${SSH}" sudo journalctl -u power-edge -n 100 --no-pager

# Check binary permissions
ssh "${SSH}" ls -la /usr/local/bin/power-edge-client

# Test binary manually
ssh "${SSH}" /usr/local/bin/power-edge-client -version
```

### Config File Issues

```bash
# Verify configs uploaded correctly
ssh "${SSH}" ls -la /etc/power-edge/

# Test config validity
ssh "${SSH}" /usr/local/bin/power-edge-client \
  -state-config=/etc/power-edge/generated-state.yaml \
  -watcher-config=/etc/power-edge/generated-watcher-config.yaml \
  -check-interval=1s
```

## Quick Reference

```bash
# One-time setup
cp .env.example .env
vim .env  # Set SSH and optionally SUDO_PASS
source .env

# Deploy new node
make init
bash scripts/init/organize-config.sh /tmp/power-edge-init-*/
make deploy NODE_CONFIG=config/nodes/<hostname>

# Update existing node
vim config/nodes/<hostname>/generated-state.yaml
make deploy NODE_CONFIG=config/nodes/<hostname>

# View metrics
curl http://<node-ip>:9100/metrics

# View logs
ssh "${SSH}" sudo journalctl -u power-edge -f

# Restart service
ssh "${SSH}" sudo systemctl restart power-edge
```

## Security Notes

### `.env` File Security

- **`.env` is gitignored** - never committed to Git
- Contains sensitive data (sudo passwords)
- Permissions should be `600` (read/write owner only)

```bash
chmod 600 .env
```

### Alternative: Use SSH Config + Passwordless Sudo

Instead of storing passwords, configure:

1. **SSH config** (`~/.ssh/config`):
```
Host t420
    HostName 10.8.0.1
    User stella
    IdentityFile ~/.ssh/id_ed25519_poweredgesports_gmail_com
```

2. **Passwordless sudo** on edge node (shown above)

Then deploy with:
```bash
make init SSH=t420
make deploy SSH=t420 NODE_CONFIG=config/nodes/stella-PowerEdge-T420
```

No `.env` file needed!

---

**Next**: Set up [central monitoring with Prometheus/Grafana](DEPLOYMENT_PLAN.md#phase-2-central-observability-tech-screen-k8s)
