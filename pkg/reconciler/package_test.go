package reconciler

import (
	"context"
	"testing"

	"github.com/power-edge/power-edge/pkg/config"
)

func TestNewPackageEnforcer(t *testing.T) {
	e := NewPackageEnforcer()

	if e.applier == nil {
		t.Error("Applier not initialized")
	}
}

func TestPackageEnforcer_Reconcile(t *testing.T) {
	tests := []struct {
		name    string
		pkg     config.PackageConfig
		mode    ReconcileMode
		wantErr bool
	}{
		{
			name: "dry-run package present",
			pkg: config.PackageConfig{
				Name:  "curl",
				State: config.PackageStatePresent,
			},
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name: "dry-run package absent",
			pkg: config.PackageConfig{
				Name:  "nonexistent-package-12345",
				State: config.PackageStateAbsent,
			},
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name: "enforce package present",
			pkg: config.PackageConfig{
				Name:  "curl",
				State: config.PackageStatePresent,
			},
			mode:    ModeEnforce,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewPackageEnforcer()
			ctx := context.Background()

			result, err := e.Reconcile(ctx, tt.pkg, tt.mode)

			// If no package manager found, skip
			if err != nil && result.Error != nil && result.Error.Error() == "no supported package manager found (apt/yum/dnf)" {
				t.Skip("No supported package manager found")
			}

			if result.ResourceType != "package" {
				t.Errorf("Expected ResourceType 'package', got '%s'", result.ResourceType)
			}

			if result.ResourceName != tt.pkg.Name {
				t.Errorf("Expected ResourceName '%s', got '%s'", tt.pkg.Name, result.ResourceName)
			}

			if tt.mode == ModeDryRun && result.DryRun != true {
				t.Error("Expected DryRun to be true in dry-run mode")
			}

			if tt.mode == ModeEnforce && result.DryRun != false {
				t.Error("Expected DryRun to be false in enforce mode")
			}
		})
	}
}

func TestPackageEnforcer_Check(t *testing.T) {
	e := NewPackageEnforcer()

	// Test checking a package that likely exists on most systems
	installed, version, err := e.Check("bash")

	if err != nil {
		t.Logf("Check() error: %v", err)
		return
	}

	t.Logf("bash installed: %v, version: %s", installed, version)
}
