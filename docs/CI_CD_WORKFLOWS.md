# CI/CD Workflows

## Overview

Power Edge has **9 GitHub Actions workflows** that automate testing, building, versioning, and deployment.

## Workflow Summary

| Workflow | Trigger | Purpose | Production Impact |
|----------|---------|---------|-------------------|
| **test.yml** | PR, push to main/develop | Unit tests + coverage | ❌ No |
| **lint.yml** | PR, push to main/develop | Code quality checks | ❌ No |
| **build.yml** | PR, push to main/develop | Multi-platform builds | ❌ No |
| **e2e-test.yml** | Non-draft PR, push to main | Integration tests | ❌ No |
| **version-check.yml** | PR to main | Validate version bump | ❌ No |
| **pre-release.yml** | Push to main (merge) | Auto pre-release | ⚠️ Test env only |
| **suggest-version.yml** | Manual | Analyze changes, suggest version | ❌ No |
| **promote-release.yml** | Manual | Promote pre-release to full release | ✅ **Yes - enables production deployment** |
| **release.yml** | Git tag push | Full release workflow | ✅ **Yes - production artifacts** |

## Workflow Details

### 1. test.yml - Unit Tests

**Triggers**:
- Push to `main` or `develop`
- Pull requests to `main` or `develop`

**Steps**:
1. Setup Go 1.22
2. Download dependencies
3. Run tests with race detector
4. Generate coverage report
5. Upload coverage artifact
6. Check coverage threshold (>50%)

**Artifacts**:
- `coverage-report` (HTML coverage report)

**Failure conditions**:
- Tests fail
- Coverage below 50%

---

### 2. lint.yml - Code Quality

**Triggers**:
- Push to `main` or `develop`
- Pull requests to `main` or `develop`

**Jobs**:
- **golangci-lint**: Comprehensive Go linting
- **go-fmt**: Check code formatting
- **go-vet**: Static analysis
- **schema-validation**: Validate YAML syntax
- **shellcheck**: Bash script linting

**Failure conditions**:
- Linter errors
- Formatting issues
- Invalid YAML
- Shell script errors

---

### 3. build.yml - Multi-Platform Builds

**Triggers**:
- Push to `main` or `develop`
- Pull requests to `main` or `develop`

**Matrix**:
- OS: `ubuntu-latest`, `macos-latest`
- Go: `1.22`

**Steps**:
1. Generate code from schemas
2. Build binary
3. Test binary execution (`-version`)
4. Upload artifacts
5. Build Docker image (ubuntu only)

**Artifacts**:
- `edge-state-exporter-{os}-{go-version}` (binaries)

---

### 4. e2e-test.yml - Integration Tests

**Triggers**:
- Push to `main`
- Non-draft PR to `main`

**Smart execution**:
- ✅ Runs on push to main
- ✅ Runs on non-draft PRs
- ⏭️  Skips on draft PRs (save CI minutes)

**Jobs**:
- **e2e-integration**: Start exporter, test metrics endpoint
- **e2e-docker**: Test Docker container execution

**Tests**:
1. Exporter starts without crashing
2. `/metrics` endpoint returns Prometheus format
3. `/health` endpoint returns healthy status
4. `/version` endpoint returns version info
5. Compliance metrics are exposed

**Failure conditions**:
- Exporter crashes on startup
- Endpoints return errors
- Missing expected metrics

---

### 5. version-check.yml - Version Validation

**Triggers**:
- Pull requests to `main`

**Steps**:
1. Analyze changed files since base branch
2. Detect breaking changes in commits
3. Determine required version bump (MAJOR/MINOR/PATCH)
4. Post comment on PR with recommendation

**Output**:
- PR comment with version suggestion
- Required bump type (MAJOR/MINOR/PATCH)
- Next steps for creating release

**Does NOT enforce** - just advisory

---

### 6. pre-release.yml - Auto Pre-Release

**Triggers**:
- Push to `main` (typically from merged PR)

**Steps**:
1. Generate pre-release version: `{latest-tag}-pre.{count}+{sha}`
2. Build artifacts with pre-release version
3. Build and push Docker image
4. Create GitHub pre-release
5. Comment on merged PR

**Example version**: `v1.2.0-pre.145+abc1234`

**Docker tags**:
- `ghcr.io/power-edge/power-edge:v1.2.0-pre.145+abc1234`
- `ghcr.io/power-edge/power-edge:pre-latest`

**Purpose**: Enable testing before production release

---

### 7. suggest-version.yml - Version Recommendation

**Triggers**:
- Manual workflow dispatch

**Inputs**:
- `pre_release_version`: Pre-release to analyze

**Analysis**:
1. Checkout pre-release commit
2. Compare to latest full release
3. Detect breaking changes (commit messages, schema diffs)
4. Check schema additions/modifications
5. Check code changes
6. Determine required bump type

**Output**:
- Recommended version (e.g., `v1.3.0`)
- Alternative versions (PATCH, MINOR, MAJOR)
- Detailed change report
- Validation checklist

**Example**:
```
Required bump: MINOR
Recommended: v1.3.0
Reason: Schema changes (new fields/types)

Alternatives:
- PATCH: v1.2.4 (not recommended - schema changed)
- MINOR: v1.3.0 (recommended)
- MAJOR: v2.0.0 (only if breaking changes)
```

**Usage**:
1. Run suggest-version workflow
2. Review recommendations
3. Use suggested version in promote workflow

---

### 8. promote-release.yml - Promote to Release

**Triggers**:
- Manual workflow dispatch

**Inputs**:
- `pre_release_version`: Pre-release to promote
- `release_version`: New release version (e.g., `v1.2.3`)
- `is_rollback`: Flag for rollback releases

**Validation**:
1. Pre-release exists
2. Release version format valid (`v{major}.{minor}.{patch}`)
3. Version doesn't already exist
4. Version bump matches changes (if not rollback)

**Steps**:
1. Checkout pre-release commit (exact same code!)
2. Rebuild with release version
3. Pull pre-release Docker image, re-tag as release
4. Tag as `latest` (if not rollback)
5. Generate release notes
6. Create GitHub release
7. Create incident issue (if rollback)

**Safety features**:
- Checksums validated
- Same commit as pre-release (no new code!)
- Rollback creates tracking issue
- Warnings for rollbacks

**Docker tags**:
- `ghcr.io/power-edge/power-edge:v1.2.3`
- `ghcr.io/power-edge/power-edge:latest` (if not rollback)

**This is the ONLY way to create production releases** ✅

---

### 9. release.yml - Full Release (Tag-Based)

**Triggers**:
- Push of tag matching `v*.*.*`

**Jobs**:
1. **validate-tag**: Ensure tag format valid, validate version bump
2. **build-artifacts**: Build for multiple platforms (Linux/macOS, x86/ARM)
3. **docker-release**: Build multi-arch Docker images
4. **create-release**: Create GitHub release with artifacts

**Matrix builds**:
- Linux AMD64
- Linux ARM64
- macOS AMD64
- macOS ARM64

**Artifacts**:
- Tarballs for each platform
- SHA256 checksums
- Docker images (ghcr.io)

**Docker tags**:
- `v1.2.3`
- `v1.2`
- `v1`
- `latest`

**Changelog**: Auto-generated from commits since last tag

---

## Workflow Relationships

```
┌─────────────────────────────────────────────────────────────┐
│                    Development Flow                          │
└─────────────────────────────────────────────────────────────┘

Feature Branch
  ↓ (open PR)
  ├─→ test.yml (unit tests)
  ├─→ lint.yml (code quality)
  └─→ build.yml (build check)

PR Ready for Review (mark as ready)
  ↓
  └─→ e2e-test.yml (integration tests)

PR to main
  ↓
  └─→ version-check.yml (suggest version bump)

Merge to main
  ↓
  └─→ pre-release.yml (auto pre-release)
       ↓
       Creates: v1.2.0-pre.145+abc1234

Deploy to test environment
  ↓ (validate in test)

Manual: suggest-version.yml
  ↓ (analyze changes)
  Recommends: v1.3.0

Manual: promote-release.yml
  ↓ (promote pre-release)
  Creates: v1.3.0 tag + release
       ↓
       └─→ release.yml (build all platforms)

Deploy to production
  ↓
  ✅ Production running v1.3.0
```

## Common Scenarios

### Scenario 1: Normal Feature Release

```bash
# 1. Create feature branch
git checkout -b feature/add-dbus-watcher

# 2. Make changes, commit
git add .
git commit -m "feat: add D-Bus watcher support"

# 3. Open PR
gh pr create --title "Add D-Bus watcher" --body "..."

# Workflows run:
# - test.yml, lint.yml, build.yml

# 4. Mark PR as ready (if draft)
# Additional workflows:
# - e2e-test.yml
# - version-check.yml (suggests MINOR bump)

# 5. Merge PR
gh pr merge 42 --squash

# Workflow runs:
# - pre-release.yml (creates v1.2.0-pre.145+abc1234)

# 6. Test pre-release
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:v1.2.0-pre.145+abc1234 \
  -n test

# 7. Get version recommendation
# GitHub Actions > suggest-version.yml > Run workflow
# Input: v1.2.0-pre.145+abc1234
# Output: Recommends v1.3.0

# 8. Promote to release
# GitHub Actions > promote-release.yml > Run workflow
# Inputs:
#   pre_release_version: v1.2.0-pre.145+abc1234
#   release_version: v1.3.0
#   is_rollback: false

# Workflow runs:
# - release.yml (builds all platforms)

# 9. Deploy to production
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:v1.3.0 \
  -n production
```

### Scenario 2: Hotfix (PATCH)

```bash
# 1. Create hotfix branch
git checkout -b hotfix/fix-memory-leak

# 2. Fix bug
git commit -m "fix: resolve memory leak in watcher"

# 3. Open PR, merge
# pre-release.yml creates: v1.3.0-pre.146+def5678

# 4. Suggest version
# Recommends: v1.3.1 (PATCH bump)

# 5. Promote
# promote-release.yml
#   pre_release_version: v1.3.0-pre.146+def5678
#   release_version: v1.3.1

# 6. Deploy immediately to production
```

### Scenario 3: Emergency Rollback

```bash
# Production is on v1.3.1, has critical bug

# 1. Find last known-good version
git tag | grep -v pre | sort -V | tail -5
# v1.3.0 was good

# 2. Find pre-release for v1.3.0
git tag | grep "v1.3.0-pre"
# v1.3.0-pre.145+abc1234

# 3. Suggest version (optional)
# Recommends: v1.3.2 or v1.3.0

# 4. Promote as rollback
# GitHub Actions > promote-release.yml
#   pre_release_version: v1.3.0-pre.145+abc1234
#   release_version: v1.3.2
#   is_rollback: true

# Creates:
# - Release v1.3.2 (same code as v1.3.0!)
# - Incident issue: "Investigate rollback to v1.3.2"

# 5. Deploy to production
kubectl set image deployment/edge-state-exporter \
  edge-state-exporter=ghcr.io/power-edge/power-edge:v1.3.2 \
  -n production

# 6. Fix issue in development
# 7. Create new release with fix
```

## Guardrails

### 1. Production Safety

❌ **Cannot deploy pre-release to production** (policy enforcement recommended)
✅ **Can only deploy full releases** (v1.2.3)

### 2. Version Validation

- version-check.yml: Advisory (comments on PR)
- promote-release.yml: Validates bump type matches changes
- release.yml: Validates tag format

### 3. Same Code Guarantee

promote-release.yml checks out **exact same commit** as pre-release:
- Pre-release tested: `abc1234`
- Release promoted: `abc1234` (same!)
- No new code introduced during promotion

### 4. Rollback Tracking

When `is_rollback=true`:
- Creates incident issue automatically
- Adds warnings to release notes
- Does NOT tag as `latest`

## Environment Variables

Workflows use these secrets:
- `GITHUB_TOKEN`: Provided automatically by GitHub
- `GHCR`: GitHub Container Registry (automatic with `GITHUB_TOKEN`)

No additional secrets required! 🎉

## Local Testing

Test workflows locally with `act`:

```bash
# Install act
brew install act

# Test unit tests workflow
act pull_request -W .github/workflows/test.yml

# Test build workflow
act pull_request -W .github/workflows/build.yml
```

## Monitoring Workflows

```bash
# View workflow runs
gh run list

# View specific workflow
gh run list --workflow=test.yml

# View logs for failed run
gh run view <run-id> --log-failed

# Re-run failed jobs
gh run rerun <run-id> --failed
```

## Summary

| Phase | Automation | Manual |
|-------|------------|--------|
| **Development** | test, lint, build, e2e | - |
| **Merge** | pre-release (auto) | - |
| **Version Planning** | - | suggest-version |
| **Release** | release.yml (on tag) | promote-release |
| **Deployment** | - | kubectl/helm/argocd |

**Key**: Testing is automatic, releases are deliberate ✅
