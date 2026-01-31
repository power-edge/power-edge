package apply

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/power-edge/power-edge/pkg/config"
)

// FirewallApplier is the single source of truth for applying firewall state (UFW)
type FirewallApplier struct{}

// NewFirewallApplier creates a new firewall applier
func NewFirewallApplier() *FirewallApplier {
	return &FirewallApplier{}
}

// Apply ensures firewall matches desired state
func (a *FirewallApplier) Apply(fw *config.FirewallConfig, dryRun bool) ApplyResult {
	result := ApplyResult{
		Actions: []string{},
	}

	if fw == nil {
		result.Changed = false
		return result
	}

	// Check if UFW is available
	if !a.isUFWInstalled() {
		result.Error = fmt.Errorf("ufw is not installed")
		return result
	}

	// Check enabled/disabled state
	isEnabled, err := a.isEnabled()
	if err != nil {
		result.Error = fmt.Errorf("failed to check UFW status: %w", err)
		return result
	}

	if fw.Enabled && !isEnabled {
		result.Changed = true
		result.Actions = append(result.Actions, "ufw enable")
		if !dryRun {
			if err := a.enable(); err != nil {
				result.Error = err
				return result
			}
		}
	} else if !fw.Enabled && isEnabled {
		result.Changed = true
		result.Actions = append(result.Actions, "ufw disable")
		if !dryRun {
			if err := a.disable(); err != nil {
				result.Error = err
				return result
			}
		}
	}

	// Apply allowed services
	if fw.Enabled && len(fw.AllowedServices) > 0 {
		for _, service := range fw.AllowedServices {
			result.Actions = append(result.Actions, fmt.Sprintf("ufw allow %s", service))
			if !dryRun {
				if err := a.allowService(service); err != nil {
					result.Error = fmt.Errorf("failed to allow service %s: %w", service, err)
					return result
				}
			}
			result.Changed = true
		}
	}

	return result
}

// Check returns current firewall state
func (a *FirewallApplier) Check() (enabled bool, err error) {
	return a.isEnabled()
}

func (a *FirewallApplier) isUFWInstalled() bool {
	_, err := exec.LookPath("ufw")
	return err == nil
}

func (a *FirewallApplier) isEnabled() (bool, error) {
	cmd := exec.Command("ufw", "status")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	return strings.Contains(string(output), "Status: active"), nil
}

func (a *FirewallApplier) enable() error {
	// Use --force to avoid interactive prompt
	cmd := exec.Command("ufw", "--force", "enable")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (output: %s)", err, string(output))
	}
	return nil
}

func (a *FirewallApplier) disable() error {
	cmd := exec.Command("ufw", "disable")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (output: %s)", err, string(output))
	}
	return nil
}

func (a *FirewallApplier) allowService(service string) error {
	cmd := exec.Command("ufw", "allow", service)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (output: %s)", err, string(output))
	}
	return nil
}
