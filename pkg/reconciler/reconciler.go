package reconciler

import (
	"context"
	"fmt"
	"log"

	"github.com/power-edge/power-edge/pkg/config"
)

// ReconcileMode controls how reconciliation behaves
type ReconcileMode string

const (
	ModeDisabled ReconcileMode = "disabled" // Only monitor, never enforce
	ModeDryRun   ReconcileMode = "dry-run"  // Log what would change, don't apply
	ModeEnforce  ReconcileMode = "enforce"  // Actively fix drift
)

// ReconcileResult represents the outcome of a reconciliation attempt
type ReconcileResult struct {
	ResourceType string
	ResourceName string
	WasCompliant bool
	Action       string // e.g., "started service", "set sysctl", "no-op"
	Error        error
	DryRun       bool
}

// Reconciler enforces desired state on the edge node
type Reconciler struct {
	mode             ReconcileMode
	serviceEnforcer  *ServiceEnforcer
	sysctlEnforcer   *SysctlEnforcer
	firewallEnforcer *FirewallEnforcer
	packageEnforcer  *PackageEnforcer
	fileEnforcer     *FileEnforcer
}

// NewReconciler creates a new reconciler with the specified mode
func NewReconciler(mode ReconcileMode) *Reconciler {
	return &Reconciler{
		mode:             mode,
		serviceEnforcer:  NewServiceEnforcer(),
		sysctlEnforcer:   NewSysctlEnforcer(),
		firewallEnforcer: NewFirewallEnforcer(),
		packageEnforcer:  NewPackageEnforcer(),
		fileEnforcer:     NewFileEnforcer(),
	}
}

// ReconcileAll runs reconciliation for all state components
func (r *Reconciler) ReconcileAll(ctx context.Context, state *config.State) ([]ReconcileResult, error) {
	if r.mode == ModeDisabled {
		log.Println("   Reconciliation disabled, skipping enforcement")
		return nil, nil
	}

	var results []ReconcileResult

	// Reconcile services
	log.Println("   Reconciling services...")
	serviceResults, err := r.ReconcileServices(ctx, state.Services)
	if err != nil {
		log.Printf("   Service reconciliation error: %v", err)
	}
	results = append(results, serviceResults...)

	// Reconcile sysctl
	log.Println("   Reconciling sysctl parameters...")
	sysctlResults, err := r.ReconcileSysctl(ctx, state.Sysctl)
	if err != nil {
		log.Printf("   Sysctl reconciliation error: %v", err)
	}
	results = append(results, sysctlResults...)

	// Reconcile firewall
	if state.Firewall.Enabled || len(state.Firewall.AllowedServices) > 0 {
		log.Println("   Reconciling firewall...")
		firewallResult, err := r.ReconcileFirewall(ctx, &state.Firewall)
		if err != nil {
			log.Printf("   Firewall reconciliation error: %v", err)
		}
		results = append(results, firewallResult)
	}

	// Reconcile packages
	if len(state.Packages) > 0 {
		log.Println("   Reconciling packages...")
		packageResults, err := r.ReconcilePackages(ctx, state.Packages)
		if err != nil {
			log.Printf("   Package reconciliation error: %v", err)
		}
		results = append(results, packageResults...)
	}

	// Reconcile files
	if len(state.Files) > 0 {
		log.Println("   Reconciling files...")
		fileResults, err := r.ReconcileFiles(ctx, state.Files)
		if err != nil {
			log.Printf("   File reconciliation error: %v", err)
		}
		results = append(results, fileResults...)
	}

	// Log summary
	r.logResults(results)

	return results, nil
}

// ReconcileServices enforces desired service state
func (r *Reconciler) ReconcileServices(ctx context.Context, services []config.ServiceConfig) ([]ReconcileResult, error) {
	var results []ReconcileResult

	for _, svc := range services {
		result, err := r.serviceEnforcer.Reconcile(ctx, svc, r.mode)
		if err != nil {
			result.Error = err
		}
		results = append(results, result)
	}

	return results, nil
}

// ReconcileSysctl enforces desired sysctl parameters
func (r *Reconciler) ReconcileSysctl(ctx context.Context, params map[string]string) ([]ReconcileResult, error) {
	var results []ReconcileResult

	for key, expectedValue := range params {
		result, err := r.sysctlEnforcer.Reconcile(ctx, key, expectedValue, r.mode)
		if err != nil {
			result.Error = err
		}
		results = append(results, result)
	}

	return results, nil
}

// ReconcileFirewall enforces desired firewall state
func (r *Reconciler) ReconcileFirewall(ctx context.Context, fw *config.FirewallConfig) (ReconcileResult, error) {
	return r.firewallEnforcer.Reconcile(ctx, fw, r.mode)
}

// ReconcilePackages enforces desired package state
func (r *Reconciler) ReconcilePackages(ctx context.Context, packages []config.PackageConfig) ([]ReconcileResult, error) {
	var results []ReconcileResult

	for _, pkg := range packages {
		result, err := r.packageEnforcer.Reconcile(ctx, pkg, r.mode)
		if err != nil {
			result.Error = err
		}
		results = append(results, result)
	}

	return results, nil
}

// ReconcileFiles enforces desired file state
func (r *Reconciler) ReconcileFiles(ctx context.Context, files []config.FileConfig) ([]ReconcileResult, error) {
	var results []ReconcileResult

	for _, file := range files {
		result, err := r.fileEnforcer.Reconcile(ctx, file, r.mode)
		if err != nil {
			result.Error = err
		}
		results = append(results, result)
	}

	return results, nil
}

// SetMode updates the reconciliation mode at runtime
func (r *Reconciler) SetMode(mode ReconcileMode) {
	log.Printf("Reconciliation mode changed: %s → %s", r.mode, mode)
	r.mode = mode
}

// GetMode returns the current reconciliation mode
func (r *Reconciler) GetMode() ReconcileMode {
	return r.mode
}

func (r *Reconciler) logResults(results []ReconcileResult) {
	compliant := 0
	enforced := 0
	failed := 0

	for _, result := range results {
		if result.Error != nil {
			failed++
			log.Printf("   ✗ %s/%s: %v", result.ResourceType, result.ResourceName, result.Error)
		} else if result.WasCompliant {
			compliant++
		} else {
			enforced++
			if result.DryRun {
				log.Printf("   🔍 [DRY-RUN] %s/%s: would execute '%s'", result.ResourceType, result.ResourceName, result.Action)
			} else {
				log.Printf("   ✓ %s/%s: %s", result.ResourceType, result.ResourceName, result.Action)
			}
		}
	}

	log.Printf("   Summary: %d compliant, %d enforced, %d failed", compliant, enforced, failed)
}

// ReconcileEvent triggers reconciliation for a specific event
func (r *Reconciler) ReconcileEvent(ctx context.Context, eventType, resourceName string, state *config.State) error {
	if r.mode == ModeDisabled {
		return nil
	}

	log.Printf("🔧 Triggered reconciliation: %s changed (%s)", resourceName, eventType)

	// For now, reconcile everything
	// TODO: Optimize to only reconcile affected resources
	_, err := r.ReconcileAll(ctx, state)
	return err
}

// HealthCheck verifies the reconciler is functioning
func (r *Reconciler) HealthCheck() error {
	if r.serviceEnforcer == nil {
		return fmt.Errorf("service enforcer not initialized")
	}
	if r.sysctlEnforcer == nil {
		return fmt.Errorf("sysctl enforcer not initialized")
	}
	return nil
}
