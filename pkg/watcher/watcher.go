package watcher

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/power-edge/power-edge/pkg/config"
)

// EventType represents the type of event
type EventType string

const (
	EventFileModified    EventType = "file_modified"
	EventServiceLog      EventType = "service_log"
	EventCommandExecuted EventType = "command_executed"
	EventUnitStateChange EventType = "unit_state_change"
)

// Event represents a system event
type Event struct {
	Type      EventType
	Source    string
	Path      string
	Unit      string
	Command   string
	Timestamp time.Time
	Data      map[string]string
}

// Reconciler interface for triggering reconciliation
type Reconciler interface {
	ReconcileEvent(ctx context.Context, eventType, resourceName string, state *config.State) error
}

// EventWatcher manages all system event watchers
type EventWatcher struct {
	config      *config.WatcherConfig
	reconciler  Reconciler
	state       *config.State
	eventChan   chan Event
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewEventWatcher creates a new event watcher
func NewEventWatcher(cfg *config.WatcherConfig, reconciler Reconciler, state *config.State) *EventWatcher {
	return &EventWatcher{
		config:     cfg,
		reconciler: reconciler,
		state:      state,
		eventChan:  make(chan Event, 100), // Buffer size from config
	}
}

// Start initializes and starts all configured watchers
func (w *EventWatcher) Start(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	if !w.config.Watchers.Enabled {
		return fmt.Errorf("watchers are disabled in config")
	}

	// Start event processor
	w.wg.Add(1)
	go w.processEvents()

	// Start inotify watcher
	if w.config.Watchers.Inotify.Enabled {
		log.Printf("   Starting inotify watcher for %d paths", len(w.config.Watchers.Inotify.Paths))
		w.wg.Add(1)
		go w.runInotifyWatcher()
	}

	// Start journald watcher
	if w.config.Watchers.Journald.Enabled {
		log.Printf("   Starting journald watcher for %d units", len(w.config.Watchers.Journald.Units))
		w.wg.Add(1)
		go w.runJournaldWatcher()
	}

	// Start auditd watcher
	if w.config.Watchers.Auditd.Enabled {
		log.Printf("   Starting auditd watcher for %d commands", len(w.config.Watchers.Auditd.Commands))
		w.wg.Add(1)
		go w.runAuditdWatcher()
	}

	// Start dbus watcher
	if w.config.Watchers.Dbus.Enabled {
		log.Printf("   Starting dbus watcher")
		w.wg.Add(1)
		go w.runDbusWatcher()
	}

	return nil
}

// Stop gracefully stops all watchers
func (w *EventWatcher) Stop() error {
	log.Println("Stopping event watchers...")
	w.cancel()
	w.wg.Wait()
	close(w.eventChan)
	log.Println("Event watchers stopped")
	return nil
}

// processEvents handles incoming events from all watchers
func (w *EventWatcher) processEvents() {
	defer w.wg.Done()

	for {
		select {
		case event := <-w.eventChan:
			w.handleEvent(event)
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *EventWatcher) handleEvent(event Event) {
	log.Printf("📨 Event: %s from %s at %s", event.Type, event.Source, event.Timestamp.Format(time.RFC3339))

	switch event.Type {
	case EventFileModified:
		log.Printf("   File modified: %s", event.Path)
		// Trigger reconciliation for file changes
		if w.reconciler != nil {
			if err := w.reconciler.ReconcileEvent(w.ctx, string(event.Type), event.Path, w.state); err != nil {
				log.Printf("   Reconciliation triggered by file change failed: %v", err)
			}
		}
	case EventServiceLog:
		log.Printf("   Service log: %s", event.Unit)
		// Parse log and trigger alerts if needed (future)
	case EventCommandExecuted:
		log.Printf("   Command executed: %s", event.Command)
		// Trigger reconciliation for commands that might affect state
		if w.reconciler != nil && w.affectsMonitoredState(event.Command) {
			if err := w.reconciler.ReconcileEvent(w.ctx, string(event.Type), event.Command, w.state); err != nil {
				log.Printf("   Reconciliation triggered by command failed: %v", err)
			}
		}
	case EventUnitStateChange:
		log.Printf("   Unit state changed: %s", event.Unit)
		// Trigger immediate reconciliation for unit state changes
		if w.reconciler != nil {
			if err := w.reconciler.ReconcileEvent(w.ctx, string(event.Type), event.Unit, w.state); err != nil {
				log.Printf("   Reconciliation triggered by unit change failed: %v", err)
			}
		}
	}
}

// affectsMonitoredState checks if a command might affect state we're monitoring
func (w *EventWatcher) affectsMonitoredState(command string) bool {
	// Commands that affect system state we care about
	stateChangingCommands := []string{
		"systemctl",
		"sysctl",
		"ufw",
		"firewall-cmd",
		"apt",
		"yum",
		"dnf",
	}

	for _, cmd := range stateChangingCommands {
		if strings.Contains(command, cmd) {
			return true
		}
	}
	return false
}

// Platform-specific watcher implementations are in:
// - watcher_linux.go (real implementations for Linux)
// - watcher_stub.go (placeholders for non-Linux platforms)
