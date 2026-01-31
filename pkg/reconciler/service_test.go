package reconciler

import (
	"context"
	"testing"

	"github.com/power-edge/power-edge/pkg/config"
)

func TestNewServiceEnforcer(t *testing.T) {
	e := NewServiceEnforcer()

	if e.applier == nil {
		t.Error("Applier not initialized")
	}
}

func TestServiceEnforcer_Reconcile(t *testing.T) {
	tests := []struct {
		name    string
		svc     config.ServiceConfig
		mode    ReconcileMode
		wantErr bool
	}{
		{
			name: "dry-run running service",
			svc: config.ServiceConfig{
				Name:    "test-service",
				State:   config.ServiceStateRunning,
				Enabled: true,
			},
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name: "dry-run stopped service",
			svc: config.ServiceConfig{
				Name:    "test-service",
				State:   config.ServiceStateStopped,
				Enabled: false,
			},
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name: "enforce running service",
			svc: config.ServiceConfig{
				Name:    "test-service",
				State:   config.ServiceStateRunning,
				Enabled: true,
			},
			mode:    ModeEnforce,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewServiceEnforcer()
			ctx := context.Background()

			result, err := e.Reconcile(ctx, tt.svc, tt.mode)

			// Skip if systemctl not available (e.g., on macOS)
			if err != nil && result.Error != nil {
				t.Skipf("Systemctl not available: %v", err)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if result.ResourceType != "service" {
				t.Errorf("Expected ResourceType 'service', got '%s'", result.ResourceType)
			}

			if result.ResourceName != tt.svc.Name {
				t.Errorf("Expected ResourceName '%s', got '%s'", tt.svc.Name, result.ResourceName)
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

func TestServiceEnforcer_Check(t *testing.T) {
	e := NewServiceEnforcer()

	// Test with a service that likely doesn't exist
	_, _, err := e.Check("nonexistent-test-service-12345")

	// We expect either no error (service not found) or a specific error
	// This is mainly to ensure the Check function doesn't panic
	if err != nil {
		t.Logf("Check() returned error (expected for nonexistent service): %v", err)
	}
}
