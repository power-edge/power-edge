package apply

import (
	"testing"
)

func TestSysctlApplier_Apply(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		dryRun   bool
		wantErr  bool
	}{
		{
			name:    "valid sysctl key in dry-run",
			key:     "net.ipv4.ip_forward",
			value:   "1",
			dryRun:  true,
			wantErr: false,
		},
		{
			name:    "another valid key in dry-run",
			key:     "vm.swappiness",
			value:   "60",
			dryRun:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewSysctlApplier()
			result := a.Apply(tt.key, tt.value, tt.dryRun)

			if (result.Error != nil) != tt.wantErr {
				t.Errorf("Apply() error = %v, wantErr %v", result.Error, tt.wantErr)
			}

			if tt.dryRun && result.Error == nil {
				// In dry-run, should not have errors for valid keys
				t.Logf("Dry-run successful for %s=%s", tt.key, tt.value)
			}
		})
	}
}

func TestSysctlApplier_Get(t *testing.T) {
	a := NewSysctlApplier()

	// Test getting a common sysctl value (should exist on most systems)
	value, err := a.Get("kernel.hostname")
	if err != nil {
		t.Skipf("Skipping test, sysctl not available: %v", err)
	}

	if value == "" {
		t.Error("Expected non-empty hostname")
	}

	t.Logf("kernel.hostname = %s", value)
}

func TestSysctlApplier_InvalidKey(t *testing.T) {
	a := NewSysctlApplier()

	// Test with completely invalid key
	_, err := a.Get("invalid.nonexistent.key.12345")
	if err == nil {
		t.Error("Expected error for invalid sysctl key")
	}
}
