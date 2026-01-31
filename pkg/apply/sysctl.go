package apply

import (
	"fmt"
	"os/exec"
	"strings"
)

// SysctlApplier is the single source of truth for applying sysctl parameters
type SysctlApplier struct{}

// NewSysctlApplier creates a new sysctl applier
func NewSysctlApplier() *SysctlApplier {
	return &SysctlApplier{}
}

// Apply ensures a sysctl parameter matches its desired value
// This is the ONLY place that knows HOW to apply sysctl state
func (a *SysctlApplier) Apply(key, desiredValue string, dryRun bool) ApplyResult {
	result := ApplyResult{
		Actions: []string{},
	}

	// Get current value
	actualValue, err := a.Get(key)
	if err != nil {
		result.Error = fmt.Errorf("failed to get sysctl value: %w", err)
		return result
	}

	// Check if change needed
	if actualValue == desiredValue {
		result.Changed = false
		return result
	}

	result.Changed = true
	result.Actions = []string{fmt.Sprintf("sysctl -w %s=%s", key, desiredValue)}

	// Dry-run mode: don't apply
	if dryRun {
		return result
	}

	// Apply change
	if err := a.Set(key, desiredValue); err != nil {
		result.Error = fmt.Errorf("failed to set sysctl value: %w", err)
		return result
	}

	return result
}

// Get retrieves the current value of a sysctl parameter
func (a *SysctlApplier) Get(key string) (string, error) {
	cmd := exec.Command("sysctl", "-n", key)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// Set applies a new value to a sysctl parameter
func (a *SysctlApplier) Set(key, value string) error {
	cmd := exec.Command("sysctl", "-w", fmt.Sprintf("%s=%s", key, value))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (output: %s)", err, string(output))
	}
	return nil
}

// SetPersistent writes sysctl changes to /etc/sysctl.d/ for persistence across reboots
func (a *SysctlApplier) SetPersistent(key, value, configFile string) error {
	// First apply runtime change
	if err := a.Set(key, value); err != nil {
		return fmt.Errorf("failed to set runtime value: %w", err)
	}

	// Then persist to config file
	// Note: This is a placeholder for future implementation
	// Would need proper file management to update /etc/sysctl.d/99-power-edge.conf
	return nil
}
