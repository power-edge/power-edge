package reconciler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/power-edge/power-edge/pkg/config"
)

func TestNewFileEnforcer(t *testing.T) {
	e := NewFileEnforcer()

	if e.applier == nil {
		t.Error("Applier not initialized")
	}
}

func TestFileEnforcer_Reconcile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		file    config.FileConfig
		mode    ReconcileMode
		wantErr bool
	}{
		{
			name: "dry-run create file",
			file: config.FileConfig{
				Path:    config.UnixPath(filepath.Join(tmpDir, "test1.txt")),
				Content: "hello world",
				Mode:    "0644",
			},
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name: "dry-run update permissions",
			file: config.FileConfig{
				Path: config.UnixPath(filepath.Join(tmpDir, "test2.txt")),
				Mode: "0755",
			},
			mode:    ModeDryRun,
			wantErr: false,
		},
		{
			name: "enforce create file",
			file: config.FileConfig{
				Path:    config.UnixPath(filepath.Join(tmpDir, "test3.txt")),
				Content: "enforced content",
				Mode:    "0644",
			},
			mode:    ModeEnforce,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewFileEnforcer()
			ctx := context.Background()

			result, err := e.Reconcile(ctx, tt.file, tt.mode)

			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if result.ResourceType != "file" {
				t.Errorf("Expected ResourceType 'file', got '%s'", result.ResourceType)
			}

			if result.ResourceName != string(tt.file.Path) {
				t.Errorf("Expected ResourceName '%s', got '%s'", tt.file.Path, result.ResourceName)
			}

			if tt.mode == ModeDryRun && result.DryRun != true {
				t.Error("Expected DryRun to be true in dry-run mode")
			}

			if tt.mode == ModeEnforce && result.DryRun != false {
				t.Error("Expected DryRun to be false in enforce mode")
			}

			// In enforce mode, verify file was created
			if tt.mode == ModeEnforce && tt.file.Content != "" {
				if _, err := os.Stat(string(tt.file.Path)); os.IsNotExist(err) {
					t.Error("File was not created in enforce mode")
				}

				// Verify content
				content, err := os.ReadFile(string(tt.file.Path))
				if err != nil {
					t.Fatalf("Failed to read file: %v", err)
				}

				if string(content) != tt.file.Content {
					t.Errorf("Content mismatch: got %q, want %q", string(content), tt.file.Content)
				}
			}
		})
	}
}

func TestFileEnforcer_Check(t *testing.T) {
	e := NewFileEnforcer()

	// Create a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := "test content"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	exists, mode, owner, group, sha256sum, err := e.Check(testFile)

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
