package apply

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/power-edge/power-edge/pkg/config"
)

// ServiceApplier is the single source of truth for applying service state
type ServiceApplier struct{}

// NewServiceApplier creates a new service applier
func NewServiceApplier() *ServiceApplier {
	return &ServiceApplier{}
}

// ApplyResult contains the outcome of applying state
type ApplyResult struct {
	Changed bool
	Actions []string
	Error   error
}

// Apply ensures a service matches its desired state
// This is the ONLY place that knows HOW to apply service state
func (a *ServiceApplier) Apply(svc config.ServiceConfig, dryRun bool) ApplyResult {
	result := ApplyResult{
		Actions: []string{},
	}

	// Check current state
	isActive, err := a.isServiceActive(svc.Name)
	if err != nil {
		result.Error = fmt.Errorf("failed to check service status: %w", err)
		return result
	}

	isEnabled, err := a.isServiceEnabled(svc.Name)
	if err != nil {
		result.Error = fmt.Errorf("failed to check service enabled status: %w", err)
		return result
	}

	// Determine required actions
	var actions []string

	// Check active/inactive state
	switch svc.State {
	case config.ServiceStateRunning:
		if !isActive {
			actions = append(actions, "start")
		}
	case config.ServiceStateStopped:
		if isActive {
			actions = append(actions, "stop")
		}
	}

	// Check enabled/disabled state
	if svc.Enabled && !isEnabled {
		actions = append(actions, "enable")
	} else if !svc.Enabled && isEnabled {
		actions = append(actions, "disable")
	}

	// No changes needed
	if len(actions) == 0 {
		result.Changed = false
		return result
	}

	result.Changed = true
	result.Actions = actions

	// Dry-run mode: don't apply
	if dryRun {
		return result
	}

	// Apply changes
	for _, action := range actions {
		if err := a.executeSystemctl(action, svc.Name); err != nil {
			result.Error = fmt.Errorf("failed to %s service: %w", action, err)
			return result
		}
	}

	return result
}

// Check returns the current state of a service
func (a *ServiceApplier) Check(name string) (isActive, isEnabled bool, err error) {
	isActive, err = a.isServiceActive(name)
	if err != nil {
		return false, false, err
	}

	isEnabled, err = a.isServiceEnabled(name)
	if err != nil {
		return false, false, err
	}

	return isActive, isEnabled, nil
}

func (a *ServiceApplier) isServiceActive(name string) (bool, error) {
	cmd := exec.Command("systemctl", "is-active", name)
	output, err := cmd.Output()
	status := strings.TrimSpace(string(output))

	// systemctl is-active returns exit code 3 if inactive (not an error for us)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 3 {
			return false, nil
		}
		return false, err
	}

	return status == "active", nil
}

func (a *ServiceApplier) isServiceEnabled(name string) (bool, error) {
	cmd := exec.Command("systemctl", "is-enabled", name)
	output, err := cmd.Output()
	status := strings.TrimSpace(string(output))

	// systemctl is-enabled returns exit code 1 if disabled
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}

	return status == "enabled", nil
}

func (a *ServiceApplier) executeSystemctl(action, serviceName string) error {
	cmd := exec.Command("systemctl", action, serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (output: %s)", err, string(output))
	}
	return nil
}
