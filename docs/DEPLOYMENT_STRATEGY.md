# Deployment Strategy

## Overview

Power Edge follows a **strict deployment pipeline** that ensures only thoroughly tested releases reach production environments.

## Deployment Environments

### Development (`dev`)
- **Trigger**: Any commit to feature branches
- **Artifacts**: Local builds only
- **Purpose**: Developer testing
- **Rules**: No restrictions

### Testing (`test`)
- **Trigger**: Pre-releases (automatic on merge to `main`)
- **Artifacts**: Docker images tagged with `{version}-pre.{count}+{sha}`
- **Purpose**: Integration testing, QA validation
- **Rules**: Can deploy pre-releases

### Production (`prod`)
- **Trigger**: Full releases (manual promotion)
- **Artifacts**: Docker images tagged with `v{major}.{minor}.{patch}`
- **Purpose**: Live production workloads
- **Rules**: ⚠️ **MUST use full releases ONLY** - no pre-releases allowed

## Release Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    Development Workflow                     │
└─────────────────────────────────────────────────────────────┘

1. Feature Branch
   └─> Build locally
   └─> Run unit tests
   └─> Open PR

2. Pull Request (non-draft)
   ├─> Run unit tests
   ├─> Run linters
   └─> Run E2E tests

3. Merge to main
   ├─> Run ALL tests
   ├─> Build artifacts
   └─> Create pre-release
       ├─> Tag: v0.0.0-pre.123+abc1234
       ├─> Docker: ghcr.io/power-edge/power-edge:v0.0.0-pre.123+abc1234
       └─> Docker: ghcr.io/power-edge/power-edge:pre-latest

4. Test in non-production
   └─> Deploy pre-release to test environment
   └─> Validate functionality
   └─> Run smoke tests

5. Promote to release (manual)
   ├─> workflow_dispatch: promote-release.yml
   ├─> Input: pre-release version
   ├─> Input: new release version (v1.2.3)
   ├─> Validate version bump
   └─> Create full release
       ├─> Tag: v1.2.3
       ├─> Docker: ghcr.io/power-edge/power-edge:v1.2.3
       ├─> Docker: ghcr.io/power-edge/power-edge:latest
       └─> GitHub Release with artifacts

6. Deploy to production
   └─> ⚠️ ONLY full releases (v1.2.3)
   └─> NEVER pre-releases
```

## Deployment Rules

### ✅ Allowed in Production

```yaml
# GOOD - Full release
image: ghcr.io/power-edge/power-edge:v1.2.3

# GOOD - Latest (points to latest full release)
image: ghcr.io/power-edge/power-edge:latest

# GOOD - Specific major.minor (gets latest patch)
image: ghcr.io/power-edge/power-edge:v1.2
```

### ❌ Forbidden in Production

```yaml
# BAD - Pre-release
image: ghcr.io/power-edge/power-edge:v0.0.0-pre.123+abc1234

# BAD - Pre-latest (untested)
image: ghcr.io/power-edge/power-edge:pre-latest

# BAD - SHA-based tags (not promoted)
image: ghcr.io/power-edge/power-edge:abc1234
```

## Version Tag Naming

### Pre-Releases (auto-generated on merge)
```
Format: v{base}-pre.{count}+{sha}
Example: v1.2.0-pre.145+abc1234

Where:
  {base}  = Latest full release tag (v1.2.0)
  {count} = Commit count since base
  {sha}   = Short commit SHA

Purpose: Testing, QA validation
Deploy to: test, staging (NEVER production)
```

### Full Releases (manual promotion)
```
Format: v{major}.{minor}.{patch}
Example: v1.2.3

Where:
  {major} = Breaking changes
  {minor} = New features (backward compatible)
  {patch} = Bug fixes, config changes

Purpose: Production deployment
Deploy to: production, customer environments
```

### Rollback Releases (manual promotion with flag)
```
Format: v{major}.{minor}.{patch} (same format, older version)
Example: v1.2.1 (when current is v1.2.3)

Purpose: Emergency rollback to known-good version
Process: Promote older pre-release with is_rollback=true
```

## Deployment Commands

### Test Environment (pre-releases OK)

```bash
# Deploy latest pre-release for testing
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:pre-latest

# Deploy specific pre-release
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:v1.2.0-pre.145+abc1234
```

### Production Environment (full releases ONLY)

```bash
# ✅ Deploy full release
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:v1.2.3

# ✅ Deploy latest full release
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:latest

# ❌ NEVER deploy pre-release to production
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:pre-latest  # FORBIDDEN
```

## Enforcement

### 1. Tag Protection

Configure GitHub repository settings:

```yaml
Tag Protection Rules:
  - Pattern: v[0-9]+.[0-9]+.[0-9]+
    Required: Manual approval
    Restrict who can create: Maintainers only
```

### 2. Kubernetes Admission Control

Use OPA or admission webhooks:

```rego
# OPA policy: Deny pre-release images in production namespace
package kubernetes.admission

deny[msg] {
  input.request.kind.kind == "Deployment"
  input.request.namespace == "production"
  container := input.request.object.spec.template.spec.containers[_]
  regex.match("-pre\\.", container.image)
  msg := sprintf("Pre-release images forbidden in production: %v", [container.image])
}
```

### 3. GitOps (ArgoCD/FluxCD)

```yaml
# ArgoCD Application
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: edge-state-exporter-prod
spec:
  source:
    targetRevision: v1.2.3  # Explicit version tag
    # NOT: pre-latest or any pre-release
  destination:
    namespace: production
```

## Rollback Procedure

### When to Rollback

- Critical bug discovered in production
- Performance degradation
- Data integrity issues
- Security vulnerability

### How to Rollback

```bash
# 1. Identify last known-good version
git tag -l "v*" | grep -v pre | sort -V | tail -5

# 2. Find the pre-release for that version
git tag -l "v1.2.1-pre*"
# Output: v1.2.1-pre.120+xyz7890

# 3. Promote old pre-release as new release
# GitHub Actions > promote-release.yml > Run workflow
# Inputs:
#   pre_release_version: v1.2.1-pre.120+xyz7890
#   release_version: v1.2.4 (or v1.2.1 if you want same version)
#   is_rollback: true

# 4. Deploy to production
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:v1.2.4
```

### Post-Rollback

1. **Create incident issue** (auto-created by workflow)
2. **Document root cause**
3. **Fix in development branch**
4. **Add regression tests**
5. **Plan next release** with fix

## Promotion Checklist

Before promoting a pre-release to production:

- [ ] All tests pass on pre-release
- [ ] Deployed to test environment successfully
- [ ] QA validation complete
- [ ] No known critical bugs
- [ ] CHANGELOG.md updated
- [ ] Version bump matches changes (MAJOR/MINOR/PATCH)
- [ ] Release notes drafted
- [ ] Rollback plan documented

## Examples

### Example 1: Normal Release Flow

```bash
# 1. Merge PR #42 to main
git merge --no-ff feature/add-dbus-watcher

# 2. Pre-release auto-created
# Tag: v1.2.0-pre.145+abc1234
# Docker: ghcr.io/power-edge/power-edge:v1.2.0-pre.145+abc1234

# 3. Deploy to test
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:v1.2.0-pre.145+abc1234 \
  -n test

# 4. Test passes, promote to v1.3.0 (MINOR bump for new watcher)
# GitHub Actions > promote-release.yml
#   pre_release_version: v1.2.0-pre.145+abc1234
#   release_version: v1.3.0
#   is_rollback: false

# 5. Deploy to production
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:v1.3.0 \
  -n production
```

### Example 2: Emergency Rollback

```bash
# Production on v1.3.0 has critical bug

# 1. Find last known-good version
git tag | grep -v pre | sort -V | tail -5
# v1.2.8
# v1.2.9
# v1.3.0  # Current, has bug

# 2. Find pre-release for v1.2.9
git tag | grep "v1.2.9-pre"
# v1.2.9-pre.140+xyz7890

# 3. Promote as rollback
# GitHub Actions > promote-release.yml
#   pre_release_version: v1.2.9-pre.140+xyz7890
#   release_version: v1.3.1 (or v1.2.9)
#   is_rollback: true

# 4. Deploy to production immediately
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:v1.3.1 \
  -n production

# 5. Incident issue auto-created: "Investigate rollback to v1.3.1"
```

## Monitoring Deployments

### Prometheus Metrics

```promql
# Track deployed versions across environments
edge_state_info{environment="production", version="v1.3.0"}
edge_state_info{environment="test", version="v1.3.0-pre.145+abc1234"}

# Alert on pre-release in production
ALERTS:
  - alert: PreReleaseInProduction
    expr: edge_state_info{environment="production", version=~".*-pre.*"}
    annotations:
      summary: "Pre-release deployed to production (FORBIDDEN)"
```

### Deployment History

```bash
# View deployment history
kubectl rollout history deployment/edge-state-exporter -n production

# Rollback to previous version (last full release)
kubectl rollout undo deployment/edge-state-exporter -n production
```

## Summary

| Environment | Allowed Versions | Purpose | Deployment Trigger |
|-------------|------------------|---------|-------------------|
| **dev** | Any | Development | Manual / local build |
| **test** | Pre-releases, Full releases | Testing, QA | Auto on merge to main |
| **production** | Full releases ONLY | Live workloads | Manual promotion |

**Key Rule**: 🚨 **Production deployments MUST use full release tags (v1.2.3) - NEVER pre-releases**

---

**Rationale**: Pre-releases are for testing. Only code that has been tested and explicitly promoted should reach production. This prevents accidental deployment of untested code and provides a clear audit trail.
