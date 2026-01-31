package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/power-edge/power-edge/pkg/config"
	"github.com/power-edge/power-edge/pkg/gitops"
	"github.com/power-edge/power-edge/pkg/metrics"
	"github.com/power-edge/power-edge/pkg/reconciler"
	"github.com/power-edge/power-edge/pkg/watcher"
)

var (
	// Build-time variables (set via -ldflags)
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func main() {
	// Flags
	stateConfig := flag.String("state-config", "/etc/power-edge/state.yaml", "Path to state configuration")
	watcherConfig := flag.String("watcher-config", "/etc/power-edge/watcher.yaml", "Path to watcher configuration")
	listenAddr := flag.String("listen", ":9100", "Prometheus metrics listen address")
	checkInterval := flag.Duration("check-interval", 30*time.Second, "State check interval")
	reconcileMode := flag.String("reconcile", "disabled", "Reconciliation mode: disabled, dry-run, enforce")
	gitopsRepo := flag.String("gitops-repo", "", "GitOps repository URL (enables GitOps sync)")
	gitopsBranch := flag.String("gitops-branch", "main", "GitOps repository branch")
	gitopsPath := flag.String("gitops-path", "state.yaml", "Path to state.yaml in GitOps repo")
	gitopsInterval := flag.Duration("gitops-interval", 30*time.Second, "GitOps sync interval")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	// Version info
	if *version {
		fmt.Printf("edge-state-exporter %s\n", Version)
		fmt.Printf("  Git Commit: %s\n", GitCommit)
		fmt.Printf("  Build Time: %s\n", BuildTime)
		os.Exit(0)
	}

	log.Printf("🚀 Starting edge-state-exporter %s", Version)
	log.Printf("   State Config:      %s", *stateConfig)
	log.Printf("   Watcher Config:    %s", *watcherConfig)
	log.Printf("   Listen Addr:       %s", *listenAddr)
	log.Printf("   Check Interval:    %s", *checkInterval)
	log.Printf("   Reconcile Mode:    %s", *reconcileMode)
	if *gitopsRepo != "" {
		log.Printf("   GitOps Repo:       %s@%s", *gitopsRepo, *gitopsBranch)
		log.Printf("   GitOps Path:       %s", *gitopsPath)
		log.Printf("   GitOps Interval:   %s", *gitopsInterval)
	}

	// Load configurations
	log.Println("📖 Loading configurations...")
	state, err := config.LoadStateConfig(*stateConfig)
	if err != nil {
		log.Fatalf("Failed to load state config: %v", err)
	}
	log.Printf("   Loaded state config: %s (%s)", state.Metadata.Site, state.Metadata.Environment)

	watcherCfg, err := config.LoadWatcherConfig(*watcherConfig)
	if err != nil {
		log.Fatalf("Failed to load watcher config: %v", err)
	}
	log.Printf("   Loaded watcher config (watchers enabled: %v)", watcherCfg.Watchers.Enabled)

	// Initialize reconciler
	var reconMode reconciler.ReconcileMode
	switch *reconcileMode {
	case "enforce":
		reconMode = reconciler.ModeEnforce
		log.Println("⚙️  Reconciliation: ENFORCE (will actively fix drift)")
	case "dry-run":
		reconMode = reconciler.ModeDryRun
		log.Println("🔍 Reconciliation: DRY-RUN (will log changes without applying)")
	default:
		reconMode = reconciler.ModeDisabled
		log.Println("👁️  Reconciliation: DISABLED (monitor-only mode)")
	}
	reconcilerInstance := reconciler.NewReconciler(reconMode)

	// Initialize metrics
	metricsCollector := metrics.NewCollector(state)

	// Initialize watchers
	var eventWatcher *watcher.EventWatcher
	if watcherCfg.Watchers.Enabled {
		log.Println("🔍 Initializing event watchers...")
		eventWatcher = watcher.NewEventWatcher(watcherCfg, reconcilerInstance, state)
		if err := eventWatcher.Start(context.Background()); err != nil {
			log.Fatalf("Failed to start watchers: %v", err)
		}
		log.Println("   ✅ Event watchers started")
	} else {
		log.Println("⚠️  Event watchers disabled")
	}

	// Initialize GitOps sync if configured
	var gitopsSync *gitops.GitOpsSync
	if *gitopsRepo != "" {
		log.Println("🔄 Initializing GitOps sync...")
		gitopsSync = gitops.NewGitOpsSync(gitops.Config{
			RepoURL:      *gitopsRepo,
			Branch:       *gitopsBranch,
			StatePath:    *gitopsPath,
			PollInterval: *gitopsInterval,
			OnUpdate: func(newState *config.State) error {
				// Update state and trigger reconciliation
				state = newState
				if reconcilerInstance.GetMode() != reconciler.ModeDisabled {
					_, err := reconcilerInstance.ReconcileAll(context.Background(), state)
					return err
				}
				return nil
			},
		})

		go func() {
			if err := gitopsSync.Start(context.Background()); err != nil {
				log.Printf("GitOps sync error: %v", err)
			}
		}()
		log.Println("   ✅ GitOps sync started")
	}

	// Start periodic state checker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runPeriodicChecks(ctx, state, metricsCollector, reconcilerInstance, *checkInterval)

	// Start HTTP server for Prometheus metrics
	http.Handle("/metrics", metricsCollector.Handler())
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/version", versionHandler)

	server := &http.Server{
		Addr:         *listenAddr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("📊 Metrics server listening on %s", *listenAddr)
		log.Printf("   /metrics - Prometheus metrics")
		log.Printf("   /health  - Health check")
		log.Printf("   /version - Version info")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("🛑 Shutting down gracefully...")

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Stop watchers
	if eventWatcher != nil {
		if err := eventWatcher.Stop(); err != nil {
			log.Printf("Watcher shutdown error: %v", err)
		}
	}

	log.Println("✅ Shutdown complete")
}

func runPeriodicChecks(ctx context.Context, state *config.State, collector *metrics.Collector, recon *reconciler.Reconciler, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial check
	log.Println("🔍 Running initial state check...")
	if err := collector.CheckAndUpdate(state); err != nil {
		log.Printf("State check error: %v", err)
	}

	// Run initial reconciliation
	if recon.GetMode() != reconciler.ModeDisabled {
		log.Println("🔧 Running initial reconciliation...")
		if _, err := recon.ReconcileAll(ctx, state); err != nil {
			log.Printf("Reconciliation error: %v", err)
		}
	}

	for {
		select {
		case <-ticker.C:
			log.Println("🔍 Running periodic state check...")
			if err := collector.CheckAndUpdate(state); err != nil {
				log.Printf("State check error: %v", err)
			}

			// Run periodic reconciliation
			if recon.GetMode() != reconciler.ModeDisabled {
				log.Println("🔧 Running periodic reconciliation...")
				if _, err := recon.ReconcileAll(ctx, state); err != nil {
					log.Printf("Reconciliation error: %v", err)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","version":"%s"}`, Version)
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"version":"%s","git_commit":"%s","build_time":"%s"}`, Version, GitCommit, BuildTime)
}
