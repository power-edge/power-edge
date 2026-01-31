package reconciler

import (
	"context"
	"log"
	"strings"

	"github.com/power-edge/power-edge/pkg/apply"
	"github.com/power-edge/power-edge/pkg/config"
)

// PackageEnforcer orchestrates WHEN to apply package state
// The actual HOW is delegated to pkg/apply
type PackageEnforcer struct {
	applier *apply.PackageApplier
}

// NewPackageEnforcer creates a new package enforcer
func NewPackageEnforcer() *PackageEnforcer {
	return &PackageEnforcer{
		applier: apply.NewPackageApplier(),
	}
}

// Reconcile detects drift and triggers applier to fix it
func (e *PackageEnforcer) Reconcile(ctx context.Context, pkg config.PackageConfig, mode ReconcileMode) (ReconcileResult, error) {
	result := ReconcileResult{
		ResourceType: "package",
		ResourceName: pkg.Name,
		DryRun:       mode == ModeDryRun,
	}

	// Use the applier to check and potentially apply state
	dryRun := (mode != ModeEnforce)
	applyResult := e.applier.Apply(pkg, dryRun)

	if applyResult.Error != nil {
		result.Error = applyResult.Error
		return result, applyResult.Error
	}

	// Already compliant
	if !applyResult.Changed {
		result.WasCompliant = true
		result.Action = "compliant"
		log.Printf("      ✓ %s: already compliant", pkg.Name)
		return result, nil
	}

	// Changes needed/applied
	result.WasCompliant = false
	result.Action = strings.Join(applyResult.Actions, " + ")

	if mode == ModeDryRun {
		log.Printf("      🔍 [DRY-RUN] %s: would execute: %s", pkg.Name, result.Action)
	} else if mode == ModeEnforce {
		log.Printf("      ✓ %s: executed '%s'", pkg.Name, result.Action)
	}

	return result, nil
}

// Check returns whether a package is installed and its version
func (e *PackageEnforcer) Check(name string) (installed bool, version string, err error) {
	return e.applier.Check(name)
}
