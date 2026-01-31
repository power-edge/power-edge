package apply

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/power-edge/power-edge/pkg/config"
)

func TestFileApplier_Apply(t *testing.T) {
	// Create temp directory for tests
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		file    config.FileConfig
		dryRun  bool
		wantErr bool
	}{
		{
			name: "create file with content",
			file: config.FileConfig{
				Path:    config.UnixPath(filepath.Join(tmpDir, "test1.txt")),
				Content: "hello world",
				Mode:    "0644",
			},
			dryRun:  true,
			wantErr: false,
		},
		{
			name: "update file permissions",
			file: config.FileConfig{
				Path: config.UnixPath(filepath.Join(tmpDir, "test2.txt")),
				Mode: "0755",
			},
			dryRun:  true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewFileApplier()
			result := a.Apply(tt.file, tt.dryRun)

			if (result.Error != nil) != tt.wantErr {
				t.Errorf("Apply() error = %v, wantErr %v", result.Error, tt.wantErr)
			}

			if tt.dryRun && result.Changed {
				t.Logf("Dry-run detected changes: %v", result.Actions)
			}
		})
	}
}

func TestFileApplier_Check(t *testing.T) {
	// Create a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := "test content"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	a := NewFileApplier()
	exists, mode, owner, group, sha256sum, err := a.Check(testFile)

	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !exists {
		t.Error("File should exist")
	}

	if mode == "" {
		t.Error("Mode should not be empty")
	}

	if sha256sum == "" {
		t.Error("SHA256 should not be empty")
	}

	t.Logf("File check: exists=%v, mode=%s, owner=%s, group=%s, sha256=%s",
		exists, mode, owner, group, sha256sum)
}

func TestFileApplier_WriteAndVerify(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "verify.txt")

	a := NewFileApplier()

	// Apply in real mode (not dry-run)
	file := config.FileConfig{
		Path:    config.UnixPath(testFile),
		Content: "verification test",
		Mode:    "0644",
	}

	result := a.Apply(file, false)
	if result.Error != nil {
		t.Fatalf("Apply() failed: %v", result.Error)
	}

	// Verify file was created
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("File was not created")
	}

	// Verify content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != "verification test" {
		t.Errorf("Content mismatch: got %q, want %q", string(content), "verification test")
	}
}
