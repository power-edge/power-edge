package reconciler

import (
	"context"
	"testing"

	"github.com/power-edge/power-edge/pkg/config"
)

func TestNewFirewallEnforcer(t *testing.T) {
	e := NewFirewallEnforcer()

	if e.applier == nil {
		t.Error("Applier not initialized")
	}
}

func TestFirewallEnforcer_Reconcile(t *testing.T) {
	tests := []struct {
		name    string
		fw      *config.FirewallConfig
		mode    ReconcileMode
		wantErr bool
	}{
		{
			name: "dry-run enabled with services",
			fw: &config.FirewallConfig{
				Enabled:         true,
				AllowedServices: []string{"ssh", "http"},
			},
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name: "dry-run disabled",
			fw: &config.FirewallConfig{
				Enabled: false,
			},
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name:    "nil firewall config",
			fw:      nil,
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name: "enforce enabled",
			fw: &config.FirewallConfig{
				Enabled:         true,
				AllowedServices: []string{"ssh"},
			},
			mode:    ModeEnforce,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewFirewallEnforcer()
			ctx := context.Background()

			result, err := e.Reconcile(ctx, tt.fw, tt.mode)

			// If UFW is not installed, skip the test
			if err != nil && result.Error != nil && result.Error.Error() == "ufw is not installed" {
				t.Skip("UFW not installed, skipping test")
			}

			if result.ResourceType != "firewall" {
				t.Errorf("Expected ResourceType 'firewall', got '%s'", result.ResourceType)
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

func TestFirewallEnforcer_Check(t *testing.T) {
	e := NewFirewallEnforcer()

	enabled, err := e.Check()

	// If UFW is not installed, that's ok for the test
	if err != nil {
		t.Logf("Check() error (UFW may not be installed): %v", err)
		return
	}

	t.Logf("UFW enabled: %v", enabled)
}
