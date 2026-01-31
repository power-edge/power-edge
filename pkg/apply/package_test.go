package apply

import (
	"testing"

	"github.com/power-edge/power-edge/pkg/config"
)

func TestPackageApplier_Apply(t *testing.T) {
	tests := []struct {
		name    string
		pkg     config.PackageConfig
		dryRun  bool
		wantErr bool
	}{
		{
			name: "package present",
			pkg: config.PackageConfig{
				Name:  "curl",
				State: config.PackageStatePresent,
			},
			dryRun:  true,
			wantErr: false,
		},
		{
			name: "package absent",
			pkg: config.PackageConfig{
				Name:  "nonexistent-package-12345",
				State: config.PackageStateAbsent,
			},
			dryRun:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewPackageApplier()
			result := a.Apply(tt.pkg, tt.dryRun)

			// If no package manager found, skip
			if result.Error != nil && result.Error.Error() == "no supported package manager found (apt/yum/dnf)" {
				t.Skip("No supported package manager found")
			}

			if (result.Error != nil) != tt.wantErr {
				t.Errorf("Apply() error = %v, wantErr %v", result.Error, tt.wantErr)
			}
		})
	}
}

func TestPackageApplier_Check(t *testing.T) {
	a := NewPackageApplier()

	// Test checking a package that likely exists on most systems
	installed, version, err := a.Check("bash")

	if err != nil {
		t.Logf("Check() error: %v", err)
		return
	}

	t.Logf("bash installed: %v, version: %s", installed, version)
}

func TestDetectPackageManager(t *testing.T) {
	pm := detectPackageManager()

	if pm == "" {
		t.Skip("No package manager detected on this system")
	}

	t.Logf("Detected package manager: %s", pm)

	validManagers := map[string]bool{
		"apt": true,
		"yum": true,
		"dnf": true,
	}

	if !validManagers[pm] {
		t.Errorf("Unexpected package manager: %s", pm)
	}
}
