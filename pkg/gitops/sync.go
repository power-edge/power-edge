package gitops

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/power-edge/power-edge/pkg/config"
)

// GitOpsSync periodically syncs state configuration from a Git repository
type GitOpsSync struct {
	repoURL      string
	branch       string
	statePath    string // Path to state.yaml within repo
	localPath    string // Local clone path
	pollInterval time.Duration
	onUpdate     func(*config.State) error // Callback when state changes
}

// Config represents GitOps sync configuration
type Config struct {
	RepoURL      string
	Branch       string
	StatePath    string        // e.g., "config/nodes/hostname/state.yaml"
	PollInterval time.Duration // e.g., 30s
	OnUpdate     func(*config.State) error
}

// NewGitOpsSync creates a new GitOps syncer
func NewGitOpsSync(cfg Config) *GitOpsSync {
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 30 * time.Second
	}

	return &GitOpsSync{
		repoURL:      cfg.RepoURL,
		branch:       cfg.Branch,
		statePath:    cfg.StatePath,
		localPath:    filepath.Join("/tmp", "power-edge-gitops"),
		pollInterval: cfg.PollInterval,
		onUpdate:     cfg.OnUpdate,
	}
}

// Start begins polling the Git repository for changes
func (g *GitOpsSync) Start(ctx context.Context) error {
	log.Printf("🔄 Starting GitOps sync: %s@%s", g.repoURL, g.branch)
	log.Printf("   Polling every %s for changes to %s", g.pollInterval, g.statePath)

	// Initial clone
	if err := g.cloneOrPull(); err != nil {
		return fmt.Errorf("initial clone failed: %w", err)
	}

	// Load initial state
	if err := g.checkAndUpdate(); err != nil {
		log.Printf("Initial state load failed: %v", err)
	}

	// Start polling loop
	ticker := time.NewTicker(g.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := g.cloneOrPull(); err != nil {
				log.Printf("GitOps sync error: %v", err)
				continue
			}

			if err := g.checkAndUpdate(); err != nil {
				log.Printf("GitOps update error: %v", err)
			}

		case <-ctx.Done():
			log.Println("GitOps sync stopped")
			return nil
		}
	}
}

func (g *GitOpsSync) cloneOrPull() error {
	// Check if repo exists
	if _, err := os.Stat(filepath.Join(g.localPath, ".git")); os.IsNotExist(err) {
		// Clone
		log.Printf("   Cloning %s...", g.repoURL)
		cmd := exec.Command("git", "clone", "--depth=1", "--branch", g.branch, g.repoURL, g.localPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git clone failed: %s (output: %s)", err, string(output))
		}
		return nil
	}

	// Pull latest
	cmd := exec.Command("git", "-C", g.localPath, "pull", "origin", g.branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %s (output: %s)", err, string(output))
	}

	// Check if anything changed
	if !containsChange(string(output)) {
		return nil
	}

	log.Printf("   ✅ Pulled latest changes from %s", g.repoURL)
	return nil
}

func (g *GitOpsSync) checkAndUpdate() error {
	stateFile := filepath.Join(g.localPath, g.statePath)

	// Check if state file exists
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		return fmt.Errorf("state file not found: %s", stateFile)
	}

	// Load state
	newState, err := config.LoadStateConfig(stateFile)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Trigger update callback
	if g.onUpdate != nil {
		log.Printf("   📝 State updated from Git, triggering reconciliation...")
		if err := g.onUpdate(newState); err != nil {
			return fmt.Errorf("update callback failed: %w", err)
		}
	}

	return nil
}

func containsChange(output string) bool {
	// Check if git pull output indicates changes
	return !(output == "Already up to date.\n" ||
		output == "Already up-to-date.\n" ||
		len(output) == 0)
}
