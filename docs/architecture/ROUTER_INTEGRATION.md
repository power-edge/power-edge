# Router Integration - Architectural Scope

## Question: Can Power Edge Control Router Settings?

**Short Answer**: Not directly from edge nodes, but potentially via central controller or separate integration.

## Current Scope

### power-edge-client (Edge Node Agent)
**Scope**: Controls **state of the node it runs on**
- ✅ Services running on the node (docker, ssh, etc.)
- ✅ Sysctl parameters on the node
- ✅ Node firewall (UFW, iptables on the node)
- ❌ Upstream router/gateway settings

**Why**: The client runs **on** the edge node, not on the router.

### Example Network Topology

```
┌─────────────────────────────────────────────────────┐
│                  Your Network                        │
├─────────────────────────────────────────────────────┤
│                                                      │
│  ┌──────────────┐         ┌───────────────────┐    │
│  │   Router     │◄────────┤  Edge Node (T420) │    │
│  │ 10.8.0.1/24  │         │  power-edge-client│    │
│  │              │         │  stella@10.8.0.1  │    │
│  └──────────────┘         └───────────────────┘    │
│        ↑                                            │
│        │ (NOT controlled by power-edge-client)     │
│        │                                            │
│  ✅ Node firewall: iptables, UFW on T420           │
│  ❌ Router settings: Port forwarding, DNS, DHCP    │
│                                                      │
└─────────────────────────────────────────────────────┘
```

**power-edge-client** controls the T420 (where it runs), not the router.

## Router Control Options

### Option 1: Router as Another Edge Node ✅

**If your router runs Linux** (OpenWRT, pfSense, DD-WRT, VyOS):

```bash
# Deploy power-edge-client to the router
make init SSH=root@router.local
make deploy SSH=root@router.local NODE_CONFIG=config/nodes/router
```

**Controls**:
- ✅ Router's firewall (iptables/nftables on router)
- ✅ Router's sysctl (IP forwarding, conntrack, etc.)
- ✅ Services running on router (dnsmasq, hostapd, etc.)

**Does NOT Control**:
- ❌ Router-specific APIs (port forwarding UI, DHCP reservations, etc.)

### Option 2: Central Controller with Router API ⏭️ Future

**Architecture**: `power-edge-server` (central controller) integrates with router APIs

```
┌───────────────────────────────────────────────────┐
│          tech-screen K8s Cluster                  │
│                                                    │
│  ┌─────────────────────────────────────────┐     │
│  │  power-edge-server (Future)             │     │
│  │  - Manages edge node fleet              │     │
│  │  - Pushes configs to power-edge-clients │     │
│  │  - Integrates with router APIs          │     │
│  └─────────────────────────────────────────┘     │
│                      ↓                            │
│              ┌───────┴────────┐                  │
│              │                │                  │
└──────────────┼────────────────┼──────────────────┘
               │                │
               ↓                ↓
         ┌─────────┐      ┌──────────┐
         │ Router  │      │ T420     │
         │ API     │      │ Client   │
         └─────────┘      └──────────┘
```

**Supported Router APIs**:
- OpenWRT: UCI API (REST via uhttpd-mod-ubus)
- pfSense: REST API (pfSense Plus)
- UniFi: UniFi Controller API
- MikroTik: RouterOS API
- DD-WRT: Web scraping (less ideal)

**Example Integrations**:

**OpenWRT**:
```go
// power-edge-server talks to router API
POST http://router.local/uci
{
  "method": "set",
  "config": "firewall",
  "section": "zone",
  "values": {
    "name": "vpn",
    "input": "ACCEPT"
  }
}
```

**UniFi**:
```go
// Via UniFi Controller API
POST https://controller.local:8443/api/s/default/rest/portforward
{
  "name": "SSH",
  "dst_port": "22",
  "fwd": "10.8.0.1",
  "fwd_port": "22"
}
```

### Option 3: Router Operator/Integration 🔮 Possible

**Separate Project**: `power-edge-router-operator` (Kubernetes operator)

```yaml
# CRD: RouterConfig
apiVersion: power-edge.io/v1
kind: RouterConfig
metadata:
  name: home-router
spec:
  provider: openwrt
  endpoint: http://192.168.1.1
  auth:
    secretRef: router-credentials
  portForwards:
    - name: ssh-to-t420
      externalPort: 2222
      internalIP: 10.8.0.1
      internalPort: 22
      protocol: tcp
  dhcpReservations:
    - hostname: stella-t420
      mac: "00:11:22:33:44:55"
      ip: 10.8.0.1
  dnsRecords:
    - hostname: t420.lan
      ip: 10.8.0.1
```

**Reconciliation Loop**:
1. Watch RouterConfig CRDs
2. Read router current state via API
3. Compare desired vs actual
4. Apply changes via API
5. Report status

## Recommendation: Start Simple

### Phase 1: Focus on Edge Nodes ✅ (Current)
- Deploy `power-edge-client` to edge nodes (T420, VPN server)
- Monitor node-level state (services, sysctl, firewall on the node)

### Phase 2: Router as Edge Node (If Applicable)
- If router runs Linux: Deploy `power-edge-client` to router
- Monitor router as another edge node

### Phase 3: Central Controller 🔮 (Future)
- Build `power-edge-server` for fleet management
- Add router API integrations as plugins

## Router Types & Control Methods

| Router Type | Deploy Client? | API Control? | Notes |
|-------------|---------------|--------------|-------|
| OpenWRT | ✅ Yes | ✅ Yes (UCI API) | Full Linux, runs power-edge-client |
| pfSense | ✅ Yes | ✅ Yes (REST API) | FreeBSD, needs Go binary for BSD |
| DD-WRT | ✅ Yes | ⚠️ Limited (web scraping) | Linux-based, client works |
| VyOS | ✅ Yes | ✅ Yes (vyos-api) | Debian-based, client works |
| UniFi Gateway | ❌ No | ✅ Yes (Controller API) | Proprietary, API only |
| Consumer (TP-Link, Netgear) | ❌ No | ⚠️ Varies | Usually web scraping only |
| MikroTik RouterOS | ⚠️ Maybe | ✅ Yes (RouterOS API) | Custom OS, needs special client |

## Example: VPN Gateway on T420

**Current Setup** (T420 is the VPN gateway):

```
Internet → Router → T420 (OpenVPN server) → VPN Clients
```

**What power-edge-client on T420 Controls**:
- ✅ OpenVPN service state
- ✅ IP forwarding (net.ipv4.ip_forward)
- ✅ iptables NAT rules for VPN
- ✅ VPN network interface monitoring

**What it Does NOT Control**:
- ❌ Router port forwarding (UDP 1194 → T420)
- ❌ Router firewall rules allowing VPN traffic
- ❌ Router DNS settings

**To Control Router**:
- **Option A**: Manually configure router port forwarding once
- **Option B**: Deploy `power-edge-client` to router (if OpenWRT/pfSense)
- **Option C**: Use router API from `power-edge-server` (future)

## Summary

| Level | Tool | Scope |
|-------|------|-------|
| **Edge Nodes** | `power-edge-client` | Services, sysctl, firewall **on the node** |
| **Router (Linux-based)** | `power-edge-client` | Treat router as another edge node |
| **Router (API-based)** | `power-edge-server` (future) | Centralized router control via APIs |
| **Fleet Management** | `power-edge-server` (future) | Push configs to all nodes + routers |

**Current Focus**: Edge nodes (T420, VPN server) with `power-edge-client`

**Future Expansion**: Central controller with router API integrations

---

**Related Docs**:
- [Identity-Driven Configuration](IDENTITY_DRIVEN_CONFIG.md)
- [Deployment Strategy](../DEPLOYMENT_STRATEGY.md)
