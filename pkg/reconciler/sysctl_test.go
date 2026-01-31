package reconciler

import (
	"context"
	"testing"
)

func TestNewSysctlEnforcer(t *testing.T) {
	e := NewSysctlEnforcer()

	if e.applier == nil {
		t.Error("Applier not initialized")
	}
}

func TestSysctlEnforcer_Reconcile(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		mode    ReconcileMode
		wantErr bool
	}{
		{
			name:    "dry-run ip_forward",
			key:     "net.ipv4.ip_forward",
			value:   "1",
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name:    "dry-run swappiness",
			key:     "vm.swappiness",
			value:   "60",
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name:    "enforce ip_forward",
			key:     "net.ipv4.ip_forward",
			value:   "1",
			mode:    ModeEnforce,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewSysctlEnforcer()
			ctx := context.Background()

			result, err := e.Reconcile(ctx, tt.key, tt.value, tt.mode)

			// Skip if sysctl not available (e.g., on macOS)
			if err != nil && result.Error != nil {
				t.Skipf("Sysctl not available: %v", err)
			}

			if result.ResourceType != "sysctl" {
				t.Errorf("Expected ResourceType 'sysctl', got '%s'", result.ResourceType)
			}

			if result.ResourceName != tt.key {
				t.Errorf("Expected ResourceName '%s', got '%s'", tt.key, result.ResourceName)
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

func TestSysctlEnforcer_Get(t *testing.T) {
	e := NewSysctlEnforcer()

	// Test getting a common sysctl value (should exist on most systems)
	value, err := e.Get("kernel.hostname")
	if err != nil {
		t.Skipf("Skipping test, sysctl not available: %v", err)
	}

	if value == "" {
		t.Error("Expected non-empty hostname")
	}

	t.Logf("kernel.hostname = %s", value)
}
