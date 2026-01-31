package reconciler

import (
	"context"
	"testing"

	"github.com/power-edge/power-edge/pkg/config"
)

func TestNewReconciler(t *testing.T) {
	r := NewReconciler(ModeDryRun)

	if r.mode != ModeDryRun {
		t.Errorf("Expected mode %s, got %s", ModeDryRun, r.mode)
	}

	if r.serviceEnforcer == nil {
		t.Error("Service enforcer not initialized")
	}

	if r.sysctlEnforcer == nil {
		t.Error("Sysctl enforcer not initialized")
	}

	if r.firewallEnforcer == nil {
		t.Error("Firewall enforcer not initialized")
	}

	if r.packageEnforcer == nil {
		t.Error("Package enforcer not initialized")
	}

	if r.fileEnforcer == nil {
		t.Error("File enforcer not initialized")
	}
}

func TestReconcileMode(t *testing.T) {
	tests := []struct {
		name string
		mode ReconcileMode
	}{
		{
			name: "disabled mode",
			mode: ModeDisabled,
		},
		{
			name: "dry-run mode",
			mode: ModeDryRun,
		},
		{
			name: "enforce mode",
			mode: ModeEnforce,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReconciler(tt.mode)

			if r.GetMode() != tt.mode {
				t.Errorf("GetMode() = %s, want %s", r.GetMode(), tt.mode)
			}

			// Test SetMode
			newMode := ModeDryRun
			r.SetMode(newMode)

			if r.GetMode() != newMode {
				t.Errorf("After SetMode(), GetMode() = %s, want %s", r.GetMode(), newMode)
			}
		})
	}
}

func TestReconcileAll_Disabled(t *testing.T) {
	r := NewReconciler(ModeDisabled)

	state := &config.State{
		Services: []config.ServiceConfig{
			{
				Name:    "test-service",
				State:   config.ServiceStateRunning,
				Enabled: true,
			},
		},
	}

	ctx := context.Background()
	results, err := r.ReconcileAll(ctx, state)

	if err != nil {
		t.Errorf("ReconcileAll() with disabled mode returned error: %v", err)
	}

	if results != nil {
		t.Errorf("ReconcileAll() with disabled mode should return nil results, got %v", results)
	}
}

func TestReconcileAll_DryRun(t *testing.T) {
	r := NewReconciler(ModeDryRun)

	tmpDir := t.TempDir()

	state := &config.State{
		Services: []config.ServiceConfig{
			{
				Name:    "test-service",
				State:   config.ServiceStateRunning,
				Enabled: true,
			},
		},
		Sysctl: map[string]string{
			"net.ipv4.ip_forward": "1",
		},
		Firewall: config.FirewallConfig{
			Enabled:         true,
			AllowedServices: []string{"ssh"},
		},
		Packages: []config.PackageConfig{
			{
				Name:  "curl",
				State: config.PackageStatePresent,
			},
		},
		Files: []config.FileConfig{
			{
				Path:    config.UnixPath(tmpDir + "/test.txt"),
				Content: "test content",
				Mode:    "0644",
			},
		},
	}

	ctx := context.Background()
	results, err := r.ReconcileAll(ctx, state)

	if err != nil {
		t.Errorf("ReconcileAll() returned error: %v", err)
	}

	// In dry-run mode, results should be returned
	if len(results) == 0 {
		t.Error("ReconcileAll() should return results in dry-run mode")
	}

	// All results should have DryRun = true or WasCompliant = true
	for _, result := range results {
		if !result.DryRun && !result.WasCompliant {
			t.Errorf("Result for %s/%s should be either dry-run or compliant", result.ResourceType, result.ResourceName)
		}
	}
}

func TestReconcileServices(t *testing.T) {
	r := NewReconciler(ModeDryRun)

	services := []config.ServiceConfig{
		{
			Name:    "test-service-1",
			State:   config.ServiceStateRunning,
			Enabled: true,
		},
		{
			Name:    "test-service-2",
			State:   config.ServiceStateStopped,
			Enabled: false,
		},
	}

	ctx := context.Background()
	results, err := r.ReconcileServices(ctx, services)

	if err != nil {
		t.Errorf("ReconcileServices() returned error: %v", err)
	}

	if len(results) != len(services) {
		t.Errorf("ReconcileServices() returned %d results, want %d", len(results), len(services))
	}

	for _, result := range results {
		if result.ResourceType != "service" {
			t.Errorf("Expected ResourceType 'service', got '%s'", result.ResourceType)
		}
	}
}

func TestReconcileSysctl(t *testing.T) {
	r := NewReconciler(ModeDryRun)

	params := map[string]string{
		"net.ipv4.ip_forward": "1",
		"vm.swappiness":       "60",
	}

	ctx := context.Background()
	results, err := r.ReconcileSysctl(ctx, params)

	if err != nil {
		t.Errorf("ReconcileSysctl() returned error: %v", err)
	}

	if len(results) != len(params) {
		t.Errorf("ReconcileSysctl() returned %d results, want %d", len(results), len(params))
	}

	for _, result := range results {
		if result.ResourceType != "sysctl" {
			t.Errorf("Expected ResourceType 'sysctl', got '%s'", result.ResourceType)
		}
	}
}

func TestReconcileFirewall(t *testing.T) {
	r := NewReconciler(ModeDryRun)

	fw := &config.FirewallConfig{
		Enabled:         true,
		AllowedServices: []string{"ssh", "http"},
	}

	ctx := context.Background()
	result, err := r.ReconcileFirewall(ctx, fw)

	// If UFW is not installed, skip the test
	if err != nil && err.Error() == "ufw is not installed" {
		t.Skip("UFW not installed, skipping test")
	}

	if err != nil {
		t.Errorf("ReconcileFirewall() returned error: %v", err)
	}

	if result.ResourceType != "firewall" {
		t.Errorf("Expected ResourceType 'firewall', got '%s'", result.ResourceType)
	}
}

func TestReconcilePackages(t *testing.T) {
	r := NewReconciler(ModeDryRun)

	packages := []config.PackageConfig{
		{
			Name:  "curl",
			State: config.PackageStatePresent,
		},
		{
			Name:  "nonexistent-package",
			State: config.PackageStateAbsent,
		},
	}

	ctx := context.Background()
	results, err := r.ReconcilePackages(ctx, packages)

	if err != nil {
		t.Errorf("ReconcilePackages() returned error: %v", err)
	}

	if len(results) != len(packages) {
		t.Errorf("ReconcilePackages() returned %d results, want %d", len(results), len(packages))
	}

	for _, result := range results {
		if result.ResourceType != "package" {
			t.Errorf("Expected ResourceType 'package', got '%s'", result.ResourceType)
		}
	}
}

func TestReconcileFiles(t *testing.T) {
	r := NewReconciler(ModeDryRun)

	tmpDir := t.TempDir()

	files := []config.FileConfig{
		{
			Path:    config.UnixPath(tmpDir + "/test1.txt"),
			Content: "test content 1",
			Mode:    "0644",
		},
		{
			Path:    config.UnixPath(tmpDir + "/test2.txt"),
			Content: "test content 2",
			Mode:    "0755",
		},
	}

	ctx := context.Background()
	results, err := r.ReconcileFiles(ctx, files)

	if err != nil {
		t.Errorf("ReconcileFiles() returned error: %v", err)
	}

	if len(results) != len(files) {
		t.Errorf("ReconcileFiles() returned %d results, want %d", len(results), len(files))
	}

	for _, result := range results {
		if result.ResourceType != "file" {
			t.Errorf("Expected ResourceType 'file', got '%s'", result.ResourceType)
		}
	}
}

func TestHealthCheck(t *testing.T) {
	r := NewReconciler(ModeDryRun)

	err := r.HealthCheck()
	if err != nil {
		t.Errorf("HealthCheck() returned error: %v", err)
	}

	// Test with nil enforcer
	r.serviceEnforcer = nil
	err = r.HealthCheck()
	if err == nil {
		t.Error("HealthCheck() should return error when service enforcer is nil")
	}
}

func TestReconcileEvent(t *testing.T) {
	r := NewReconciler(ModeDryRun)

	state := &config.State{
		Services: []config.ServiceConfig{
			{
				Name:    "test-service",
				State:   config.ServiceStateRunning,
				Enabled: true,
			},
		},
	}

	ctx := context.Background()
	err := r.ReconcileEvent(ctx, "file_modified", "/etc/test.conf", state)

	if err != nil {
		t.Errorf("ReconcileEvent() returned error: %v", err)
	}

	// Test with disabled mode
	r.SetMode(ModeDisabled)
	err = r.ReconcileEvent(ctx, "file_modified", "/etc/test.conf", state)

	if err != nil {
		t.Errorf("ReconcileEvent() with disabled mode returned error: %v", err)
	}
}

func TestReconcileResult(t *testing.T) {
	result := ReconcileResult{
		ResourceType: "service",
		ResourceName: "nginx",
		WasCompliant: false,
		Action:       "started service",
		Error:        nil,
		DryRun:       true,
	}

	if result.ResourceType != "service" {
		t.Errorf("Expected ResourceType 'service', got '%s'", result.ResourceType)
	}

	if result.ResourceName != "nginx" {
		t.Errorf("Expected ResourceName 'nginx', got '%s'", result.ResourceName)
	}

	if result.WasCompliant {
		t.Error("Expected WasCompliant to be false")
	}

	if result.Action != "started service" {
		t.Errorf("Expected Action 'started service', got '%s'", result.Action)
	}

	if result.Error != nil {
		t.Errorf("Expected no error, got %v", result.Error)
	}

	if !result.DryRun {
		t.Error("Expected DryRun to be true")
	}
}
