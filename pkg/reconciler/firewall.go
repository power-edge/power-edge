package reconciler

import (
	"context"
	"log"
	"strings"

	"github.com/power-edge/power-edge/pkg/apply"
	"github.com/power-edge/power-edge/pkg/config"
)

// FirewallEnforcer orchestrates WHEN to apply firewall state
// The actual HOW is delegated to pkg/apply
type FirewallEnforcer struct {
	applier *apply.FirewallApplier
}

// NewFirewallEnforcer creates a new firewall enforcer
func NewFirewallEnforcer() *FirewallEnforcer {
	return &FirewallEnforcer{
		applier: apply.NewFirewallApplier(),
	}
}

// Reconcile detects drift and triggers applier to fix it
func (e *FirewallEnforcer) Reconcile(ctx context.Context, fw *config.FirewallConfig, mode ReconcileMode) (ReconcileResult, error) {
	result := ReconcileResult{
		ResourceType: "firewall",
		ResourceName: "ufw",
		DryRun:       mode == ModeDryRun,
	}

	if fw == nil {
		result.WasCompliant = true
		result.Action = "not configured"
		return result, nil
	}

	// Use the applier to check and potentially apply state
	dryRun := (mode != ModeEnforce)
	applyResult := e.applier.Apply(fw, dryRun)

	if applyResult.Error != nil {
		result.Error = applyResult.Error
		return result, applyResult.Error
	}

	// Already compliant
	if !applyResult.Changed {
		result.WasCompliant = true
		result.Action = "compliant"
		log.Printf("      ✓ firewall: already compliant")
		return result, nil
	}

	// Changes needed/applied
	result.WasCompliant = false
	result.Action = strings.Join(applyResult.Actions, "; ")

	if mode == ModeDryRun {
		log.Printf("      🔍 [DRY-RUN] firewall: would execute: %s", result.Action)
	} else if mode == ModeEnforce {
		log.Printf("      ✓ firewall: applied %d changes", len(applyResult.Actions))
	}

	return result, nil
}

// Check returns the current firewall state without applying changes
func (e *FirewallEnforcer) Check() (enabled bool, err error) {
	return e.applier.Check()
}
