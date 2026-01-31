package reconciler

import (
	"context"
	"log"
	"strings"

	"github.com/power-edge/power-edge/pkg/apply"
	"github.com/power-edge/power-edge/pkg/config"
)

// FileEnforcer orchestrates WHEN to apply file state
// The actual HOW is delegated to pkg/apply
type FileEnforcer struct {
	applier *apply.FileApplier
}

// NewFileEnforcer creates a new file enforcer
func NewFileEnforcer() *FileEnforcer {
	return &FileEnforcer{
		applier: apply.NewFileApplier(),
	}
}

// Reconcile detects drift and triggers applier to fix it
func (e *FileEnforcer) Reconcile(ctx context.Context, file config.FileConfig, mode ReconcileMode) (ReconcileResult, error) {
	result := ReconcileResult{
		ResourceType: "file",
		ResourceName: string(file.Path),
		DryRun:       mode == ModeDryRun,
	}

	// Use the applier to check and potentially apply state
	dryRun := (mode != ModeEnforce)
	applyResult := e.applier.Apply(file, dryRun)

	if applyResult.Error != nil {
		result.Error = applyResult.Error
		return result, applyResult.Error
	}

	// Already compliant
	if !applyResult.Changed {
		result.WasCompliant = true
		result.Action = "compliant"
		log.Printf("      ✓ %s: already compliant", file.Path)
		return result, nil
	}

	// Changes needed/applied
	result.WasCompliant = false
	result.Action = strings.Join(applyResult.Actions, " + ")

	if mode == ModeDryRun {
		log.Printf("      🔍 [DRY-RUN] %s: would execute: %s", file.Path, result.Action)
	} else if mode == ModeEnforce {
		log.Printf("      ✓ %s: executed '%s'", file.Path, result.Action)
	}

	return result, nil
}

// Check returns current file state without applying changes
func (e *FileEnforcer) Check(path string) (exists bool, mode, owner, group, sha256sum string, err error) {
	return e.applier.Check(path)
}
