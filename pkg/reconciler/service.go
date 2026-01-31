package reconciler

import (
	"context"
	"log"
	"strings"

	"github.com/power-edge/power-edge/pkg/apply"
	"github.com/power-edge/power-edge/pkg/config"
)

// ServiceEnforcer orchestrates WHEN to apply service state
// The actual HOW is delegated to pkg/apply
type ServiceEnforcer struct {
	applier *apply.ServiceApplier
}

// NewServiceEnforcer creates a new service enforcer
func NewServiceEnforcer() *ServiceEnforcer {
	return &ServiceEnforcer{
		applier: apply.NewServiceApplier(),
	}
}

// Reconcile detects drift and triggers applier to fix it
func (e *ServiceEnforcer) Reconcile(ctx context.Context, svc config.ServiceConfig, mode ReconcileMode) (ReconcileResult, error) {
	result := ReconcileResult{
		ResourceType: "service",
		ResourceName: svc.Name,
		DryRun:       mode == ModeDryRun,
	}

	// Use the applier to check and potentially apply state
	dryRun := (mode != ModeEnforce)
	applyResult := e.applier.Apply(svc, dryRun)

	if applyResult.Error != nil {
		result.Error = applyResult.Error
		return result, applyResult.Error
	}

	// Already compliant
	if !applyResult.Changed {
		result.WasCompliant = true
		result.Action = "compliant"
		log.Printf("      ✓ %s: already compliant", svc.Name)
		return result, nil
	}

	// Changes needed/applied
	result.WasCompliant = false
	result.Action = strings.Join(applyResult.Actions, " + ")

	if mode == ModeDryRun {
		log.Printf("      🔍 [DRY-RUN] %s: would execute: systemctl %s", svc.Name, result.Action)
	} else if mode == ModeEnforce {
		log.Printf("      ✓ %s: executed 'systemctl %s'", svc.Name, result.Action)
	}

	return result, nil
}

// Check returns the current state without applying changes
func (e *ServiceEnforcer) Check(name string) (isActive, isEnabled bool, err error) {
	return e.applier.Check(name)
}
