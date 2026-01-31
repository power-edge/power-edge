package apply

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/power-edge/power-edge/pkg/config"
)

// PackageApplier is the single source of truth for applying package state
type PackageApplier struct {
	packageManager string // "apt", "yum", "dnf"
}

// NewPackageApplier creates a new package applier (auto-detects package manager)
func NewPackageApplier() *PackageApplier {
	pm := detectPackageManager()
	return &PackageApplier{
		packageManager: pm,
	}
}

// Apply ensures a package matches its desired state
func (a *PackageApplier) Apply(pkg config.PackageConfig, dryRun bool) ApplyResult {
	result := ApplyResult{
		Actions: []string{},
	}

	if a.packageManager == "" {
		result.Error = fmt.Errorf("no supported package manager found (apt/yum/dnf)")
		return result
	}

	// Check if package is installed
	isInstalled, installedVersion, err := a.isInstalled(pkg.Name)
	if err != nil {
		result.Error = fmt.Errorf("failed to check package status: %w", err)
		return result
	}

	// Determine required action based on desired state
	switch pkg.State {
	case config.PackageStatePresent:
		if !isInstalled {
			result.Changed = true
			result.Actions = append(result.Actions, fmt.Sprintf("%s install %s", a.packageManager, pkg.Name))
			if !dryRun {
				if err := a.install(pkg.Name, pkg.Version); err != nil {
					result.Error = err
					return result
				}
			}
		} else if pkg.Version != "" && installedVersion != pkg.Version {
			result.Changed = true
			result.Actions = append(result.Actions, fmt.Sprintf("%s install %s=%s", a.packageManager, pkg.Name, pkg.Version))
			if !dryRun {
				if err := a.install(pkg.Name, pkg.Version); err != nil {
					result.Error = err
					return result
				}
			}
		}

	case config.PackageStateAbsent:
		if isInstalled {
			result.Changed = true
			result.Actions = append(result.Actions, fmt.Sprintf("%s remove %s", a.packageManager, pkg.Name))
			if !dryRun {
				if err := a.remove(pkg.Name); err != nil {
					result.Error = err
					return result
				}
			}
		}

	case config.PackageStateLatest:
		if !isInstalled {
			result.Changed = true
			result.Actions = append(result.Actions, fmt.Sprintf("%s install %s", a.packageManager, pkg.Name))
			if !dryRun {
				if err := a.install(pkg.Name, ""); err != nil {
					result.Error = err
					return result
				}
			}
		} else {
			// Check if update available (simplified - just try to upgrade)
			result.Actions = append(result.Actions, fmt.Sprintf("%s upgrade %s", a.packageManager, pkg.Name))
			if !dryRun {
				if err := a.upgrade(pkg.Name); err != nil {
					result.Error = err
					return result
				}
			}
			result.Changed = true
		}
	}

	return result
}

// Check returns whether a package is installed and its version
func (a *PackageApplier) Check(name string) (installed bool, version string, err error) {
	return a.isInstalled(name)
}

func detectPackageManager() string {
	managers := []string{"apt", "dnf", "yum"}
	for _, mgr := range managers {
		if _, err := exec.LookPath(mgr); err == nil {
			return mgr
		}
	}
	return ""
}

func (a *PackageApplier) isInstalled(name string) (bool, string, error) {
	switch a.packageManager {
	case "apt":
		return a.isInstalledApt(name)
	case "yum", "dnf":
		return a.isInstalledYum(name)
	default:
		return false, "", fmt.Errorf("unsupported package manager: %s", a.packageManager)
	}
}

func (a *PackageApplier) isInstalledApt(name string) (bool, string, error) {
	cmd := exec.Command("dpkg-query", "-W", "-f=${Status} ${Version}", name)
	output, err := cmd.Output()
	if err != nil {
		// Package not installed
		return false, "", nil
	}

	parts := strings.Fields(string(output))
	if len(parts) >= 4 && parts[2] == "installed" {
		return true, parts[3], nil
	}

	return false, "", nil
}

func (a *PackageApplier) isInstalledYum(name string) (bool, string, error) {
	cmd := exec.Command("rpm", "-q", name)
	output, err := cmd.Output()
	if err != nil {
		// Package not installed
		return false, "", nil
	}

	// Parse version from rpm output (e.g., "nginx-1.20.1-1.el8.x86_64")
	version := strings.TrimSpace(string(output))
	return true, version, nil
}

func (a *PackageApplier) install(name, version string) error {
	var cmd *exec.Cmd

	packageSpec := name
	if version != "" {
		packageSpec = fmt.Sprintf("%s=%s", name, version)
	}

	switch a.packageManager {
	case "apt":
		cmd = exec.Command("apt-get", "install", "-y", packageSpec)
	case "yum":
		cmd = exec.Command("yum", "install", "-y", packageSpec)
	case "dnf":
		cmd = exec.Command("dnf", "install", "-y", packageSpec)
	default:
		return fmt.Errorf("unsupported package manager: %s", a.packageManager)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (output: %s)", err, string(output))
	}
	return nil
}

func (a *PackageApplier) remove(name string) error {
	var cmd *exec.Cmd

	switch a.packageManager {
	case "apt":
		cmd = exec.Command("apt-get", "remove", "-y", name)
	case "yum":
		cmd = exec.Command("yum", "remove", "-y", name)
	case "dnf":
		cmd = exec.Command("dnf", "remove", "-y", name)
	default:
		return fmt.Errorf("unsupported package manager: %s", a.packageManager)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (output: %s)", err, string(output))
	}
	return nil
}

func (a *PackageApplier) upgrade(name string) error {
	var cmd *exec.Cmd

	switch a.packageManager {
	case "apt":
		cmd = exec.Command("apt-get", "install", "--only-upgrade", "-y", name)
	case "yum":
		cmd = exec.Command("yum", "update", "-y", name)
	case "dnf":
		cmd = exec.Command("dnf", "upgrade", "-y", name)
	default:
		return fmt.Errorf("unsupported package manager: %s", a.packageManager)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (output: %s)", err, string(output))
	}
	return nil
}
