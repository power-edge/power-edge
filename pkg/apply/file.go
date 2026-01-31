package apply

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/power-edge/power-edge/pkg/config"
)

// FileApplier is the single source of truth for applying file state
type FileApplier struct{}

// NewFileApplier creates a new file applier
func NewFileApplier() *FileApplier {
	return &FileApplier{}
}

// Apply ensures a file matches its desired state
func (a *FileApplier) Apply(file config.FileConfig, dryRun bool) ApplyResult {
	result := ApplyResult{
		Actions: []string{},
	}

	// Convert UnixPath to string
	path := string(file.Path)

	// Check if file exists
	exists, err := a.exists(path)
	if err != nil {
		result.Error = fmt.Errorf("failed to check file existence: %w", err)
		return result
	}

	// Handle content if specified
	if file.Content != "" {
		if !exists || a.needsContentUpdate(path, file.Content, file.SHA256) {
			result.Changed = true
			result.Actions = append(result.Actions, fmt.Sprintf("write content to %s", path))
			if !dryRun {
				if err := a.writeContent(path, file.Content); err != nil {
					result.Error = fmt.Errorf("failed to write content: %w", err)
					return result
				}
			}
		}
	}

	// Handle permissions if specified
	if file.Mode != "" {
		currentMode, err := a.getMode(path)
		if err != nil && exists {
			result.Error = fmt.Errorf("failed to get file mode: %w", err)
			return result
		}

		if exists && currentMode != file.Mode {
			result.Changed = true
			result.Actions = append(result.Actions, fmt.Sprintf("chmod %s %s", file.Mode, path))
			if !dryRun {
				if err := a.setMode(path, file.Mode); err != nil {
					result.Error = fmt.Errorf("failed to set mode: %w", err)
					return result
				}
			}
		}
	}

	// Handle ownership if specified
	if file.Owner != "" || file.Group != "" {
		currentOwner, currentGroup, err := a.getOwnership(path)
		if err != nil && exists {
			result.Error = fmt.Errorf("failed to get ownership: %w", err)
			return result
		}

		owner := file.Owner
		if owner == "" {
			owner = "root"
		}
		group := file.Group
		if group == "" {
			group = "root"
		}

		if exists && (currentOwner != owner || currentGroup != group) {
			result.Changed = true
			result.Actions = append(result.Actions, fmt.Sprintf("chown %s:%s %s", owner, group, path))
			if !dryRun {
				if err := a.setOwnership(path, owner, group); err != nil {
					result.Error = fmt.Errorf("failed to set ownership: %w", err)
					return result
				}
			}
		}
	}

	return result
}

// Check returns current file state
func (a *FileApplier) Check(path string) (exists bool, mode, owner, group, sha256sum string, err error) {
	exists, err = a.exists(path)
	if err != nil || !exists {
		return false, "", "", "", "", err
	}

	mode, err = a.getMode(path)
	if err != nil {
		return true, "", "", "", "", err
	}

	owner, group, err = a.getOwnership(path)
	if err != nil {
		return true, mode, "", "", "", err
	}

	sha256sum, err = a.getSHA256(path)
	if err != nil {
		return true, mode, owner, group, "", err
	}

	return true, mode, owner, group, sha256sum, nil
}

func (a *FileApplier) exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (a *FileApplier) needsContentUpdate(path, content, expectedSHA256 string) bool {
	// If SHA256 is specified, check that
	if expectedSHA256 != "" {
		actualSHA256, err := a.getSHA256(path)
		if err != nil {
			return true
		}
		return actualSHA256 != expectedSHA256
	}

	// Otherwise compare content directly
	actualContent, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	return string(actualContent) != content
}

func (a *FileApplier) writeContent(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func (a *FileApplier) getMode(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	mode := info.Mode().Perm()
	return fmt.Sprintf("%04o", mode), nil
}

func (a *FileApplier) setMode(path, mode string) error {
	// Parse octal mode string
	modeInt, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return fmt.Errorf("invalid mode format: %s", mode)
	}

	return os.Chmod(path, os.FileMode(modeInt))
}

func (a *FileApplier) getOwnership(path string) (owner, group string, err error) {
	// Use stat command to get owner/group (cross-platform approach)
	cmd := exec.Command("stat", "-c", "%U %G", path)
	output, err := cmd.Output()
	if err != nil {
		// Try BSD stat format (macOS)
		cmd = exec.Command("stat", "-f", "%Su %Sg", path)
		output, err = cmd.Output()
		if err != nil {
			return "", "", err
		}
	}

	parts := strings.Fields(string(output))
	if len(parts) >= 2 {
		return parts[0], parts[1], nil
	}
	return "", "", fmt.Errorf("failed to parse ownership")
}

func (a *FileApplier) setOwnership(path, owner, group string) error {
	cmd := exec.Command("chown", fmt.Sprintf("%s:%s", owner, group), path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s (output: %s)", err, string(output))
	}
	return nil
}

func (a *FileApplier) getSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
