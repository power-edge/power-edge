package apply

import (
	"testing"

	"github.com/power-edge/power-edge/pkg/config"
)

func TestServiceApplier_Apply(t *testing.T) {
	tests := []struct {
		name    string
		svc     config.ServiceConfig
		dryRun  bool
		wantErr bool
	}{
		{
			name: "running service configuration",
			svc: config.ServiceConfig{
				Name:    "test-service",
				State:   config.ServiceStateRunning,
				Enabled: true,
			},
			dryRun:  true,
			wantErr: false,
		},
		{
			name: "stopped service configuration",
			svc: config.ServiceConfig{
				Name:    "test-service",
				State:   config.ServiceStateStopped,
				Enabled: false,
			},
			dryRun:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewServiceApplier()
			result := a.Apply(tt.svc, tt.dryRun)

			if (result.Error != nil) != tt.wantErr {
				t.Errorf("Apply() error = %v, wantErr %v", result.Error, tt.wantErr)
			}

			if tt.dryRun && result.Changed && len(result.Actions) == 0 {
				t.Errorf("Apply() in dry-run with changes should have actions")
			}
		})
	}
}

func TestServiceApplier_Check(t *testing.T) {
	a := NewServiceApplier()

	// Test checking a service that likely doesn't exist
	_, _, err := a.Check("nonexistent-test-service-12345")

	// We expect either no error (service not found) or a specific error
	// This is mainly to ensure the Check function doesn't panic
	if err != nil {
		t.Logf("Check() returned error (expected for nonexistent service): %v", err)
	}
}

func TestApplyResult_Structure(t *testing.T) {
	result := ApplyResult{
		Changed: true,
		Actions: []string{"systemctl start test"},
		Error:   nil,
	}

	if !result.Changed {
		t.Error("ApplyResult.Changed should be true")
	}

	if len(result.Actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(result.Actions))
	}

	if result.Error != nil {
		t.Errorf("Expected no error, got %v", result.Error)
	}
}
