# Identity-Driven Configuration

## Philosophy

Power Edge uses **identity-driven configuration** - describing **WHAT a node IS**, not just what services run on it. This creates semantic, self-documenting configurations that map directly to the node's purpose and workload.

## Two Approaches Compared

### Generic State Management (`state.yaml`)

**Purpose**: Low-level system state enforcement
**Focus**: Services, sysctls, packages, files
**Abstraction**: OS-level primitives

```yaml
# Generic approach - WHAT services are running
services:
  - name: openvpn@server
    state: running
    enabled: true

sysctl:
  net.ipv4.ip_forward: "1"
  net.bridge.bridge-nf-call-iptables: "1"
```

**Pros**:
- Direct mapping to OS primitives
- Easy to validate with systemctl/sysctl
- Simple to implement checkers

**Cons**:
- Doesn't express WHY (What's the purpose of IP forwarding?)
- No semantic meaning (What role does this node serve?)
- Hard to understand at a glance

### Identity-Driven Configuration (`identity.yaml`)

**Purpose**: Semantic node identity and workload modeling
**Focus**: Roles, capabilities, purpose
**Abstraction**: Business/infrastructure concepts

```yaml
# Identity approach - WHAT this node IS
node:
  hostname: stella-PowerEdge-T420
  purpose: "VPN Gateway & Container Host"
  tags: [vpn-gateway, docker-host, home-lab]

roles:
  - name: vpn-gateway
    enabled: true

vpn_gateway:
  provider: openvpn
  routing:
    ip_forward: true        # Why: Required for VPN routing
    masquerade: true        # Why: NAT for VPN clients

network_tuning:
  bridge_nf_call_iptables: true  # Why: Docker bridge networking
```

**Pros**:
- Self-documenting (purpose is clear from structure)
- Semantic meaning (VPN gateway, not just "service running")
- Easier to reason about at infrastructure level
- Maps to business/operational concepts
- Discoverable (tags, roles make querying easier)

**Cons**:
- Requires translation layer to OS primitives
- More complex schema
- Need to maintain semantic→primitive mapping

## Hybrid Approach

Power Edge uses **both** approaches:

### 1. Identity Configuration (`identity.yaml`)
**Use for**: Understanding WHAT the node IS and WHY
- Node purpose and roles
- Semantic configuration (VPN gateway, container host)
- Hardware capabilities
- Infrastructure-level decisions

### 2. State Configuration (`state.yaml`)
**Use for**: Validation and enforcement
- Derived from identity configuration
- OS-level primitives for checking
- Used by state checker/reconciler

## Example: Dell PowerEdge T420

### Identity Configuration
```yaml
node:
  hostname: stella-PowerEdge-T420
  purpose: "VPN Gateway & Container Host"
  hardware:
    model: "Dell PowerEdge T420"
    cpu_cores: 24
    memory_gb: 251

roles:
  - name: vpn-gateway
    enabled: true
  - name: container-host
    enabled: true

vpn_gateway:
  provider: openvpn
  server_config:
    network: "10.8.0.0/24"
  routing:
    ip_forward: true
    masquerade: true

container_host:
  runtime: docker
```

### Derived State Configuration
```yaml
# Auto-generated from identity.yaml
services:
  - name: openvpn@server
    state: running
    enabled: true
  - name: docker
    state: running
    enabled: true

sysctl:
  net.ipv4.ip_forward: "1"          # FROM: vpn_gateway.routing.ip_forward
  net.bridge.bridge-nf-call-iptables: "1"  # FROM: container_host.runtime=docker
```

## Code Generation Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                   Identity Configuration                        │
│                   (identity.yaml)                              │
│                                                                 │
│  • Semantic roles (vpn-gateway, container-host)               │
│  • Business-level config (VPN network, routing)               │
│  • Hardware metadata                                           │
└────────────────────────────┬────────────────────────────────────┘
                            │
                            │ Translation Layer
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    State Configuration                          │
│                    (state.yaml)                                │
│                                                                 │
│  • OS primitives (services, sysctl, packages)                 │
│  • Validation rules                                            │
│  • Checker implementations                                     │
└────────────────────────────┬────────────────────────────────────┘
                            │
                            │ Checker
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    System State                                 │
│                                                                 │
│  • Actual running services (systemctl)                        │
│  • Actual kernel parameters (sysctl)                          │
│  • Actual packages (dpkg/rpm)                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Benefits of Identity-Driven Config

### 1. Self-Documentation
```yaml
# Clear purpose
purpose: "VPN Gateway & Container Host"

# Discoverable via tags
tags: [vpn-gateway, docker-host, home-lab]
```

### 2. Infrastructure Querying
```bash
# Find all VPN gateways
grep -r "role: vpn-gateway" config/nodes/

# Find all nodes in home-lab
grep -r "tags:.*home-lab" config/nodes/
```

### 3. Semantic Validation
```go
// Check if VPN gateway has proper routing
if node.HasRole("vpn-gateway") {
    if !node.VPNGateway.Routing.IPForward {
        return errors.New("VPN gateway requires IP forwarding")
    }
}
```

### 4. Role-Based Configuration
```yaml
# Different configs for different roles
roles:
  - name: vpn-gateway
    enabled: true
    config:
      provider: openvpn      # Role-specific

  - name: k8s-node
    enabled: false
    config:
      cluster: production    # Would be different if enabled
```

### 5. Multi-AZ Consistency

When treating nodes as "personal AZs":

```yaml
# Personal dev AZ (T420)
node:
  hostname: stella-PowerEdge-T420
  purpose: "Development Environment AZ"
  tags: [dev-az, single-node, home-lab]

# Production AZ (Galleon)
node:
  hostname: oil-rig-01
  purpose: "Production Edge AZ"
  tags: [prod-az, edge-compute, galleon]

# Same roles, different scale
roles:
  - name: container-host
    enabled: true
  - name: monitoring-target
    enabled: true
```

## Implementation Strategy

### Phase 1: Identity Schema (Current)
- ✅ Define `node-identity.schema.yaml`
- ✅ Create identity configs for discovered nodes
- Document role→service mappings

### Phase 2: Translation Layer
- Build identity→state translator
- Generate `state.yaml` from `identity.yaml`
- Validate generated state configs

### Phase 3: Identity Checker
- Check node conforms to declared identity
- Validate role prerequisites
- Check semantic consistency

### Phase 4: Fleet Management
- Query fleet by roles/tags
- Roll out changes to role groups
- Identity-based alerting

## Example Use Cases

### Use Case 1: Add New Container Workload

**Identity Config** (user-friendly):
```yaml
container_host:
  workloads:
    - name: prometheus
      image: prom/prometheus:latest
      ports:
        - host: 9090
          container: 9090
          protocol: tcp
```

**Generated State** (for validation):
```yaml
services:
  - name: docker
    state: running

# Auto-added firewall rule
firewall:
  rules:
    - port: 9090
      proto: tcp
      action: allow
      comment: "Container: prometheus"
```

### Use Case 2: Migrate VPN Provider

**Identity Config** (semantic change):
```yaml
vpn_gateway:
  provider: wireguard  # Changed from openvpn
  server_config:
    network: "10.8.0.0/24"
```

**Generated State** (automatic translation):
```yaml
services:
  - name: wireguard
    state: running
  - name: openvpn@server
    state: stopped      # Auto-removed
```

### Use Case 3: Fleet-Wide Network Tuning

**Query** by role:
```bash
# Find all VPN gateways
power-edge fleet query --role=vpn-gateway
```

**Update** all matching nodes:
```yaml
# Applied to all with role: vpn-gateway
network_tuning:
  tcp_keepalive_time: 3600  # Changed from 7200
```

## Best Practices

### DO: Use Identity Config for Infrastructure Decisions

```yaml
# Good: Semantic, explains WHY
vpn_gateway:
  routing:
    ip_forward: true
    masquerade: true
```

### DON'T: Put Low-Level Details in Identity

```yaml
# Bad: Too low-level for identity config
sysctl:
  net.ipv4.ip_forward: "1"
```

### DO: Tag Nodes with Semantic Labels

```yaml
# Good: Queryable, meaningful
tags:
  - vpn-gateway
  - docker-host
  - monitoring-target
  - home-lab
```

### DON'T: Use Generic Tags

```yaml
# Bad: Not meaningful
tags:
  - linux
  - server
  - t420
```

### DO: Define Roles with Clear Purpose

```yaml
# Good: Clear role and config
roles:
  - name: vpn-gateway
    enabled: true
    config:
      provider: openvpn
      ha: false
```

### DON'T: Overload Single Role

```yaml
# Bad: Too many responsibilities
roles:
  - name: everything
    enabled: true
```

## Future Enhancements

1. **Auto-Discovery**: Infer identity from discovered state
2. **Role Templates**: Reusable role definitions
3. **Identity Validation**: Check role prerequisites (e.g., k8s-node requires min 2GB RAM)
4. **Fleet Queries**: SQL-like queries across node identities
5. **Role Dependencies**: Auto-enable dependent roles (e.g., monitoring-target requires node-exporter)

---

**Key Insight**: Identity-driven configuration makes infrastructure **understandable** and **maintainable** by expressing the **semantic purpose** of each node, not just the mechanics of what's running.
