# Semantic Versioning Strategy

## Overview

Power Edge follows strict semantic versioning (`MAJOR.MINOR.PATCH`) with special rules for schema-driven projects. The versioning strategy ensures that configuration changes don't require recompilation unless absolutely necessary.

## Version Format

```
MAJOR.MINOR.PATCH
  │     │     │
  │     │     └─ Bug fixes, config-only changes (backward compatible)
  │     └─────── New features, schema additions (requires recompile, backward compatible)
  └───────────── Breaking changes, schema modifications (requires recompile, NOT backward compatible)
```

## Versioning Rules

### MAJOR Version (Breaking Changes)

**Increment when**:
- **Schema breaking changes**: Rename fields, remove types, change field types
- **API breaking changes**: Remove endpoints, change request/response format
- **Config breaking changes**: Remove config options, change config structure
- **Behavior breaking changes**: Change defaults that affect existing deployments

**Examples**:
```yaml
# MAJOR: Renamed field (breaks existing configs)
# v1.x.x
metadata:
  site: "hostname"

# v2.0.0
metadata:
  node_name: "hostname"  # BREAKING: 'site' renamed to 'node_name'

# MAJOR: Removed field (breaks existing configs)
# v1.x.x
services:
  - name: docker
    state: running
    restart_policy: always  # Field exists

# v2.0.0
services:
  - name: docker
    state: running
    # BREAKING: 'restart_policy' removed

# MAJOR: Changed enum values (breaks existing configs)
# v1.x.x
environment: [development, production]

# v2.0.0
environment: [dev, prod]  # BREAKING: values changed
```

### MINOR Version (New Features)

**Increment when**:
- **New schema types**: Add new structs, enums, or fields (backward compatible)
- **New watchers**: Add inotify, auditd, or other watcher types
- **New checkers**: Add validation logic for new state types
- **New features**: Add functionality that requires code changes

**Requires**: Recompile (`make build`)
**Config changes**: Optional (old configs still work)

**Examples**:
```yaml
# MINOR: New optional field (old configs work without it)
# v1.2.0 -> v1.3.0
services:
  - name: docker
    state: running
    health_check:  # NEW optional field
      enabled: true
      interval: 30s

# MINOR: New watcher type (requires new Go code)
# v1.2.0 -> v1.3.0
watchers:
  dbus:  # NEW watcher type
    enabled: true
    signals:
      - "org.freedesktop.systemd1.Manager.UnitNew"

# MINOR: New checker implementation
# v1.2.0 -> v1.3.0
packages:  # NEW state type, requires new checker code
  - name: curl
    state: present
```

### PATCH Version (Bug Fixes & Config Changes)

**Increment when**:
- **Bug fixes**: Fix logic errors, memory leaks, race conditions
- **Config-only changes**: Modify node configs without schema changes
- **Documentation updates**: Update README, docs, comments
- **Dependency updates**: Update Go modules (if no API changes)
- **Performance improvements**: Optimize without behavior changes

**Does NOT require**: Recompile (config changes only)
**Config changes**: Can be deployed without rebuilding binary

**Examples**:
```yaml
# PATCH: Config value change (no code change)
# v1.2.3 -> v1.2.4
sysctl:
  vm.swappiness: "60"   # v1.2.3
  vm.swappiness: "10"   # v1.2.4 (just changed value)

# PATCH: Added new service to monitor (no schema change)
# v1.2.3 -> v1.2.4
services:
  - name: docker
    state: running
  - name: prometheus  # NEW service (schema already supports it)
    state: running

# PATCH: Bug fix in checker logic
# v1.2.3: Bug - systemctl check doesn't handle 'failed' state
# v1.2.4: Fixed - now properly detects 'failed' state
```

## CI/CD Validation

GitHub Actions validates that version bumps match the types of changes made:

```yaml
# .github/workflows/version-check.yml
- name: Validate version bump
  run: |
    # Detect what changed
    if git diff --name-only HEAD^ | grep -q '^schemas/'; then
      # Schema changed - require MINOR or MAJOR bump
      echo "Schema changed - MINOR or MAJOR version required"
    elif git diff --name-only HEAD^ | grep -q '^config/'; then
      # Config changed - PATCH is OK
      echo "Config only changed - PATCH version OK"
    fi
```

## Version Bump Decision Tree

```
                    ┌─────────────────────────┐
                    │  What changed?          │
                    └───────────┬─────────────┘
                                │
                ┌───────────────┼───────────────┐
                │               │               │
                ▼               ▼               ▼
         Schema Changed?  Code Changed?   Config Changed?
                │               │               │
                │               │               │
        ┌───────┴────────┐      │               │
        │                │      │               │
        ▼                ▼      ▼               ▼
    Breaking?      New Field?  New Feature?  Values Only?
        │                │      │               │
        ▼                ▼      ▼               ▼
      MAJOR           MINOR    MINOR          PATCH
    (rename,         (add      (new           (change
     remove,         optional  watcher,       sysctl
     type change)    field)    checker)       value)
```

## Version Management

### Checking Current Version

```bash
# From git tags
git describe --tags --always

# From built binary
./bin/edge-state-exporter -version

# From Makefile
make version
```

### Creating a New Release

```bash
# 1. Determine version bump type
#    - MAJOR: Breaking schema changes
#    - MINOR: New features, new schema fields
#    - PATCH: Bug fixes, config changes

# 2. Update version in relevant files (if needed)
#    Note: We use git tags, so no hardcoded version files

# 3. Commit changes
git add .
git commit -m "feat: add new watcher type for D-Bus"

# 4. Create tag
git tag -a v1.3.0 -m "Release v1.3.0: Add D-Bus watcher support"

# 5. Push with tags
git push origin main --tags

# 6. GitHub Actions will automatically:
#    - Run tests
#    - Build binary
#    - Create Docker image
#    - Create GitHub release
```

### Automated Versioning

Use conventional commits for automatic version bumps:

```bash
# PATCH bump
git commit -m "fix: correct systemctl state detection"
git commit -m "docs: update README with examples"

# MINOR bump
git commit -m "feat: add D-Bus watcher support"
git commit -m "feat(schema): add packages state type"

# MAJOR bump
git commit -m "feat!: rename metadata.site to metadata.node_name"
git commit -m "BREAKING CHANGE: removed restart_policy field"
```

## Configuration Versioning

Each config file specifies schema version:

```yaml
# config/nodes/stella-PowerEdge-T420/state.yaml
version: "1.0"  # Schema version, not release version

metadata:
  site: "stella-PowerEdge-T420"
```

**Schema version vs Release version**:
- **Schema version** (`version: "1.0"`): Major.Minor of schema format
- **Release version** (git tag `v1.2.3`): Full semantic version of binary

**Compatibility**:
```
edge-state-exporter v1.2.3 supports schema version 1.0
edge-state-exporter v1.5.0 supports schema version 1.0
edge-state-exporter v2.0.0 supports schema version 2.0  # BREAKING
```

## Migration Guide

### Upgrading PATCH Versions

**No recompile needed** - just update configs:

```bash
# Update config values
vim config/nodes/stella-PowerEdge-T420/state.yaml

# Restart exporter (picks up new config)
systemctl restart edge-state-exporter

# No need to rebuild binary
```

### Upgrading MINOR Versions

**Recompile required** - new features available:

```bash
# Pull latest code
git fetch origin
git checkout v1.3.0

# Rebuild (includes new watchers/checkers)
make build

# Update systemd service
make install
systemctl restart edge-state-exporter

# Optionally: Update configs to use new features
vim config/nodes/stella-PowerEdge-T420/watcher.yaml
# Add new dbus watcher configuration
```

### Upgrading MAJOR Versions

**Breaking changes** - migration required:

```bash
# Read CHANGELOG for breaking changes
cat CHANGELOG.md | grep "## [2.0.0]"

# Update schemas first
vim schemas/state.schema.yaml
# Apply breaking changes (rename fields, etc.)

# Update all node configs to match new schema
vim config/nodes/*/state.yaml
# Migrate old field names to new ones

# Rebuild with new schema
make clean
make build

# Test with new config
./bin/edge-state-exporter -state-config=config/nodes/test/state.yaml

# Deploy
make install
systemctl restart edge-state-exporter
```

## Schema Evolution

### Adding a New Optional Field (MINOR)

```yaml
# schemas/state.schema.yaml - v1.2.0 -> v1.3.0

services:
  items:
    properties:
      name:
        type: string
      state:
        type: string
      health_check:  # NEW optional field
        type: object
        properties:
          enabled:
            type: boolean
          interval:
            type: string
```

**Config migration**: Optional - old configs work as-is

### Renaming a Field (MAJOR)

```yaml
# schemas/state.schema.yaml - v1.x.x -> v2.0.0

# OLD (v1.x.x)
metadata:
  properties:
    site:
      type: string

# NEW (v2.0.0)
metadata:
  properties:
    node_name:  # BREAKING: renamed from 'site'
      type: string
```

**Config migration**: Required - update all configs:

```bash
# Migration script
find config/nodes -name "state.yaml" -exec sed -i 's/site:/node_name:/g' {} \;
```

## Version Compatibility Matrix

| Schema Version | Binary Version  | Compatible? | Notes |
|----------------|-----------------|-------------|-------|
| 1.0            | v1.0.0 - v1.9.9 | ✅ Yes      | MINOR/PATCH backward compatible |
| 1.0            | v2.0.0+         | ❌ No       | MAJOR breaking change |
| 2.0            | v2.0.0 - v2.9.9 | ✅ Yes      | New schema version |

## Best Practices

### DO:
- ✅ **PATCH** for config value changes (no schema change)
- ✅ **MINOR** for new optional fields (backward compatible)
- ✅ **MAJOR** for field renames or removals (breaking)
- ✅ Test version compatibility in CI/CD
- ✅ Document breaking changes in CHANGELOG
- ✅ Provide migration scripts for MAJOR bumps

### DON'T:
- ❌ Change field names in MINOR version
- ❌ Remove fields in MINOR version
- ❌ Require recompile for PATCH version
- ❌ Break backward compatibility without MAJOR bump
- ❌ Skip version validation in CI/CD

## Examples from Real Projects

### Terraform Provider AWS (Reference)

Power Edge follows similar patterns:

```
terraform-provider-aws v4.x -> v5.0
- MAJOR: Removed deprecated resources
- Requires: Update HCL configs, update provider version

terraform-provider-aws v4.65.0 -> v4.66.0
- MINOR: New resource types (aws_vpclattice_*)
- Requires: Update provider version (old configs work)

terraform-provider-aws v4.65.0 -> v4.65.1
- PATCH: Bug fixes
- Requires: Update provider version (no config changes)
```

## Summary

| Change Type | Version Bump | Recompile? | Config Migration? | Example |
|-------------|--------------|------------|-------------------|---------|
| Schema field rename | MAJOR | Yes | Required | `site` → `node_name` |
| Schema field removal | MAJOR | Yes | Required | Remove `restart_policy` |
| New optional field | MINOR | Yes | Optional | Add `health_check` |
| New watcher type | MINOR | Yes | Optional | Add D-Bus watcher |
| Config value change | PATCH | No | Optional | Change `swappiness: 60` → `10` |
| Bug fix | PATCH | Yes | No | Fix state detection |
| Documentation | PATCH | No | No | Update README |

---

**Key Principle**: Configuration changes (PATCH) should NEVER require recompilation. Features (MINOR) and breaking changes (MAJOR) require recompilation.
