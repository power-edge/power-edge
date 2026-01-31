package reconciler

import (
	"context"
	"fmt"
	"log"

	"github.com/power-edge/power-edge/pkg/apply"
)

// SysctlEnforcer orchestrates WHEN to apply sysctl parameters
// The actual HOW is delegated to pkg/apply
type SysctlEnforcer struct {
	applier *apply.SysctlApplier
}

// NewSysctlEnforcer creates a new sysctl enforcer
func NewSysctlEnforcer() *SysctlEnforcer {
	return &SysctlEnforcer{
		applier: apply.NewSysctlApplier(),
	}
}

// Reconcile detects drift and triggers applier to fix it
func (e *SysctlEnforcer) Reconcile(ctx context.Context, key, expectedValue string, mode ReconcileMode) (ReconcileResult, error) {
	result := ReconcileResult{
		ResourceType: "sysctl",
		ResourceName: key,
		DryRun:       mode == ModeDryRun,
	}

	// Get current value for logging
	actualValue, err := e.applier.Get(key)
	if err != nil {
		result.Error = fmt.Errorf("failed to get current value: %w", err)
		return result, result.Error
	}

	// Use the applier to check and potentially apply state
	dryRun := (mode != ModeEnforce)
	applyResult := e.applier.Apply(key, expectedValue, dryRun)

	if applyResult.Error != nil {
		result.Error = applyResult.Error
		return result, applyResult.Error
	}

	// Already compliant
	if !applyResult.Changed {
		result.WasCompliant = true
		result.Action = "compliant"
		log.Printf("      ✓ %s: already compliant (%s)", key, actualValue)
		return result, nil
	}

	// Changes needed/applied
	result.WasCompliant = false
	result.Action = fmt.Sprintf("sysctl -w %s=%s", key, expectedValue)

	if mode == ModeDryRun {
		log.Printf("      🔍 [DRY-RUN] %s: would set to %s (current: %s)", key, expectedValue, actualValue)
	} else if mode == ModeEnforce {
		log.Printf("      ✓ %s: set to %s (was: %s)", key, expectedValue, actualValue)
	}

	return result, nil
}

// Get returns the current value of a sysctl parameter
func (e *SysctlEnforcer) Get(key string) (string, error) {
	return e.applier.Get(key)
}
