package apply

import (
	"testing"

	"github.com/power-edge/power-edge/pkg/config"
)

func TestFirewallApplier_Apply(t *testing.T) {
	tests := []struct {
		name    string
		fw      *config.FirewallConfig
		dryRun  bool
		wantErr bool
	}{
		{
			name: "firewall enabled with services",
			fw: &config.FirewallConfig{
				Enabled:         true,
				AllowedServices: []string{"ssh", "http"},
			},
			dryRun:  true,
			wantErr: false,
		},
		{
			name: "firewall disabled",
			fw: &config.FirewallConfig{
				Enabled: false,
			},
			dryRun:  true,
			wantErr: false,
		},
		{
			name:    "nil firewall config",
			fw:      nil,
			dryRun:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewFirewallApplier()
			result := a.Apply(tt.fw, tt.dryRun)

			// If UFW is not installed, skip the test
			if result.Error != nil && result.Error.Error() == "ufw is not installed" {
				t.Skip("UFW not installed, skipping test")
			}

			if (result.Error != nil) != tt.wantErr {
				t.Errorf("Apply() error = %v, wantErr %v", result.Error, tt.wantErr)
			}
		})
	}
}

func TestFirewallApplier_Check(t *testing.T) {
	a := NewFirewallApplier()

	enabled, err := a.Check()

	// If UFW is not installed, that's ok for the test
	if err != nil {
		t.Logf("Check() error (UFW may not be installed): %v", err)
		return
	}

	t.Logf("UFW enabled: %v", enabled)
}
