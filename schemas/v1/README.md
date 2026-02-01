# Power Edge Schema Versioning (v1)

## Overview

This directory contains **version 1** of the Power Edge control plane schemas and target system schemas. Schemas define the data models and validation rules for managing edge node state.

## Structure

```
v1/
├── README.md                    # This file
├── targets/                     # Target system schemas (version-specific)
│   ├── ufw/
│   │   ├── 0.36.schema.yaml     # UFW 0.36.x commands and validation
│   │   └── latest.schema.yaml → 0.36.schema.yaml
│   ├── systemd/
│   │   ├── 255.schema.yaml      # systemd 255 commands
│   │   └── latest.schema.yaml → 255.schema.yaml
│   ├── iptables/
│   │   ├── 1.8.schema.yaml      # iptables 1.8.x commands
│   │   └── latest.schema.yaml → 1.8.schema.yaml
│   └── docker/
│       ├── 28.schema.yaml       # Docker Engine 28.x commands
│       └── latest.schema.yaml → 28.schema.yaml
```

## Schema Versioning Strategy

### Control Plane Versioning

The `v1/` directory represents the **control plane API version**. This is analogous to Kubernetes API versioning (`v1`, `v1beta1`, etc.).

- **v1**: Stable API, backward-compatible changes only
- **v2** (future): Breaking changes to control plane data model

### Target System Versioning

Each target system (UFW, systemd, Docker, etc.) has **version-specific schemas** that map to the installed version on the target node.

**Schema Selection Algorithm:**

```
1. Exact match:    ufw 0.36.2 → targets/ufw/0.36.2.schema.yaml (if exists)
2. Minor match:    ufw 0.36.2 → targets/ufw/0.36.schema.yaml   ✓ USE THIS
3. Major match:    ufw 0.36.2 → targets/ufw/0.schema.yaml      (if exists)
4. Latest + warn:  ufw 0.36.2 → targets/ufw/latest.schema.yaml (with warning log)
5. Error:          ufw 2.0.0 → ERROR (only 0.x schemas available)
```

**Example:**
- Discovered: `ufw 0.36.2`
- Schema selected: `v1/targets/ufw/0.36.schema.yaml`
- Validation: Minor version match (0.36.x)

### Version Discovery

The discovery process captures system versions in `system-versions.json`:

```json
{
  "discovered_at": "2026-02-01T17:45:42Z",
  "versions": {
    "kernel": "6.14.0-35-generic",
    "systemd": "255",
    "ufw": "0.36.2",
    "iptables": "1.8.10",
    "docker": "28.0.1"
  }
}
```

The reconciler uses these versions to select appropriate target schemas.

## Target System Schema Format

Each target schema defines:

### 1. Metadata
```yaml
metadata:
  target_system: ufw
  version_pattern: "0.36.*"
  compatible_versions: ["0.36.0", "0.36.1", "0.36.2"]
  description: "UFW version 0.36.x command mappings"
```

### 2. Commands
Version-specific commands for checking and modifying system state:

```yaml
commands:
  check_status:
    command: "sudo ufw status"
    expect_active: "Status: active"

  enable:
    command: "sudo ufw --force enable"
    idempotent: true
```

### 3. Reconciliation Logic
Steps to reconcile desired state with actual state:

```yaml
reconciliation:
  order:
    - check_status
    - set_defaults
    - sync_rules
    - enable_if_needed
```

### 4. Known Issues
Version-specific bugs and workarounds:

```yaml
known_issues:
  - issue: "Comments may not persist across UFW reload"
    affected_versions: ["0.36.0", "0.36.1"]
    workaround: "Re-add rules with comments after reload"
```

## Adding New Target System Versions

When a new version is detected:

1. **Check compatibility**: Can existing schema work?
   - If yes → update `compatible_versions` list
   - If no → create new version schema

2. **Create new schema**:
   ```bash
   cp v1/targets/ufw/0.36.schema.yaml v1/targets/ufw/0.40.schema.yaml
   # Edit commands/behavior for 0.40 specifics
   ```

3. **Update latest** (if this is the newest):
   ```bash
   ln -sf 0.40.schema.yaml v1/targets/ufw/latest.schema.yaml
   ```

4. **Test reconciliation** on node with new version

5. **Document changes** in schema's `known_issues` or `changelog`

## Schema Evolution

### Backward-Compatible Changes (v1)
- Add new target system schemas
- Add new commands to existing schemas
- Expand `compatible_versions` lists
- Add new optional fields

### Breaking Changes (v2)
- Change control plane data model structure
- Remove required fields
- Change field semantics
- Incompatible Redis schema changes

When breaking changes are needed, create `v2/` directory and migration guide.

## Redis Key Versioning

Redis keys include schema version to enable parallel operation during migrations:

```
v1:nodes:{node-id}:state          # Uses v1 schemas
v1:nodes:{node-id}:versions       # System versions for schema selection
v1:nodes:{node-id}:compliance     # Compliance status

v2:nodes:{node-id}:state          # Future v2 migration
```

## Integration with GitOps

GitOps syncs node state from:
```
data/nodes/{node-id}/state.yaml
```

The schema version is specified in the state file:
```yaml
version: "1.0"  # Maps to v1/ schemas
metadata:
  site: "node-name"
  ...
```

## Testing Schemas

Test schema compatibility:

```bash
# Discover current versions on target
./scripts/init/init-node.sh user@host

# Check system-versions.json
cat /tmp/power-edge-init-{node}/system-versions.json

# Verify schema exists
ls schemas/v1/targets/ufw/$(jq -r '.versions.ufw' system-versions.json | cut -d. -f1-2).schema.yaml
```

## Future Enhancements

1. **Automated schema generation**: Parse man pages and `--help` output to generate schemas
2. **Schema validation tools**: CLI to validate state.yaml against current schemas
3. **Version compatibility matrix**: Document which target versions work with which control plane versions
4. **Schema diff tools**: Compare schemas across versions to understand changes
5. **Migration scripts**: Auto-migrate state between schema versions

## References

- **Kubernetes API Versioning**: https://kubernetes.io/docs/reference/using-api/#api-versioning
- **Terraform Provider Versioning**: https://developer.hashicorp.com/terraform/language/providers/requirements
- **JSON Schema**: https://json-schema.org/

---

**Made with ❤️ for production-grade edge infrastructure management**
