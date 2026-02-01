//go:build linux
// +build linux

package watcher

import (
	"bufio"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"
	"github.com/fsnotify/fsnotify"
	"github.com/godbus/dbus/v5"
)

func (w *EventWatcher) runInotifyWatcher() {
	defer w.wg.Done()

	if len(w.config.Watchers.Inotify.Paths) == 0 {
		log.Println("   [inotify] No paths configured, skipping")
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("   [inotify] Failed to create watcher: %v", err)
		return
	}
	defer watcher.Close()

	// Add all configured paths
	for _, path := range w.config.Watchers.Inotify.Paths {
		if err := watcher.Add(path); err != nil {
			log.Printf("   [inotify] Failed to watch %s: %v", path, err)
		} else {
			log.Printf("   [inotify] Watching: %s", path)
		}
	}

	log.Println("   [inotify] Watcher started")

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Only trigger on Write and Create events
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				w.eventChan <- Event{
					Type:      EventFileModified,
					Source:    "inotify",
					Path:      event.Name,
					Timestamp: time.Now(),
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("   [inotify] Error: %v", err)
		case <-w.ctx.Done():
			log.Println("   [inotify] Watcher stopped")
			return
		}
	}
}

func (w *EventWatcher) runJournaldWatcher() {
	defer w.wg.Done()

	if len(w.config.Watchers.Journald.Units) == 0 {
		log.Println("   [journald] No units configured, skipping")
		return
	}

	journal, err := sdjournal.NewJournal()
	if err != nil {
		log.Printf("   [journald] Failed to open journal: %v", err)
		return
	}
	defer journal.Close()

	// Add match for each configured unit
	for _, unit := range w.config.Watchers.Journald.Units {
		if err := journal.AddMatch("_SYSTEMD_UNIT=" + unit + ".service"); err != nil {
			log.Printf("   [journald] Failed to add match for %s: %v", unit, err)
		} else {
			log.Printf("   [journald] Watching unit: %s", unit)
		}
	}

	// Seek to end to only get new entries
	if err := journal.SeekTail(); err != nil {
		log.Printf("   [journald] Failed to seek to tail: %v", err)
		return
	}

	log.Println("   [journald] Watcher started")

	for {
		select {
		case <-w.ctx.Done():
			log.Println("   [journald] Watcher stopped")
			return
		default:
			// Wait for new entries
			r := journal.Wait(1 * time.Second)
			if r < 0 {
				log.Printf("   [journald] Error waiting for entries")
				continue
			}

			// Read new entries
			for {
				n, err := journal.Next()
				if err != nil {
					log.Printf("   [journald] Error reading entry: %v", err)
					break
				}
				if n == 0 {
					break
				}

				entry, err := journal.GetEntry()
				if err != nil {
					log.Printf("   [journald] Error getting entry: %v", err)
					continue
				}

				// Check for state changes
				unit := entry.Fields["_SYSTEMD_UNIT"]
				message := entry.Fields["MESSAGE"]

				if strings.Contains(message, "Started") ||
				   strings.Contains(message, "Stopped") ||
				   strings.Contains(message, "Failed") ||
				   strings.Contains(message, "Reloaded") {
					w.eventChan <- Event{
						Type:      EventUnitStateChange,
						Source:    "journald",
						Unit:      unit,
						Timestamp: time.Unix(0, int64(entry.RealtimeTimestamp)*1000),
						Data: map[string]string{
							"message": message,
						},
					}
				}
			}
		}
	}
}

func (w *EventWatcher) runAuditdWatcher() {
	defer w.wg.Done()

	if len(w.config.Watchers.Auditd.Commands) == 0 {
		log.Println("   [auditd] No commands configured, skipping")
		return
	}

	// Check if auditd is available
	auditLogPath := "/var/log/audit/audit.log"
	if _, err := os.Stat(auditLogPath); os.IsNotExist(err) {
		log.Printf("   [auditd] Audit log not found at %s, using journald for command execution", auditLogPath)
		// Fall back to monitoring via journald for command executions
		w.runAuditdViaJournald()
		return
	}

	log.Printf("   [auditd] Monitoring commands: %v", w.config.Watchers.Auditd.Commands)
	log.Println("   [auditd] Watcher started (using audit log)")

	file, err := os.Open(auditLogPath)
	if err != nil {
		log.Printf("   [auditd] Failed to open audit log: %v", err)
		return
	}
	defer file.Close()

	// Seek to end
	file.Seek(0, io.SeekEnd)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Read new lines
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				// Check if line contains any of our monitored commands
				for _, cmd := range w.config.Watchers.Auditd.Commands {
					if strings.Contains(line, cmd) && strings.Contains(line, "EXECVE") {
						w.eventChan <- Event{
							Type:      EventCommandExecuted,
							Source:    "auditd",
							Command:   cmd,
							Timestamp: time.Now(),
							Data: map[string]string{
								"audit_line": line,
							},
						}
					}
				}
			}
		case <-w.ctx.Done():
			log.Println("   [auditd] Watcher stopped")
			return
		}
	}
}

func (w *EventWatcher) runAuditdViaJournald() {
	log.Println("   [auditd-fallback] Using journald to monitor command executions")

	journal, err := sdjournal.NewJournal()
	if err != nil {
		log.Printf("   [auditd-fallback] Failed to open journal: %v", err)
		return
	}
	defer journal.Close()

	// Monitor audit messages in journald
	journal.AddMatch("_TRANSPORT=audit")

	if err := journal.SeekTail(); err != nil {
		log.Printf("   [auditd-fallback] Failed to seek to tail: %v", err)
		return
	}

	log.Println("   [auditd-fallback] Watcher started")

	for {
		select {
		case <-w.ctx.Done():
			log.Println("   [auditd-fallback] Watcher stopped")
			return
		default:
			r := journal.Wait(1 * time.Second)
			if r < 0 {
				continue
			}

			for {
				n, err := journal.Next()
				if err != nil || n == 0 {
					break
				}

				entry, err := journal.GetEntry()
				if err != nil {
					continue
				}

				message := entry.Fields["MESSAGE"]
				for _, cmd := range w.config.Watchers.Auditd.Commands {
					if strings.Contains(message, cmd) {
						w.eventChan <- Event{
							Type:      EventCommandExecuted,
							Source:    "auditd-fallback",
							Command:   cmd,
							Timestamp: time.Unix(0, int64(entry.RealtimeTimestamp)*1000),
							Data: map[string]string{
								"message": message,
							},
						}
					}
				}
			}
		}
	}
}

func (w *EventWatcher) runDbusWatcher() {
	defer w.wg.Done()

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		log.Printf("   [dbus] Failed to connect to system bus: %v", err)
		return
	}
	defer conn.Close()

	// Subscribe to systemd manager signals
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/systemd1"),
		dbus.WithMatchInterface("org.freedesktop.systemd1.Manager"),
	); err != nil {
		log.Printf("   [dbus] Failed to add match signal: %v", err)
		return
	}

	signals := make(chan *dbus.Signal, 10)
	conn.Signal(signals)

	log.Println("   [dbus] Watcher started (monitoring systemd D-Bus signals)")

	for {
		select {
		case signal := <-signals:
			if signal == nil {
				continue
			}

			// Handle UnitNew, UnitRemoved, JobNew, JobRemoved signals
			switch signal.Name {
			case "org.freedesktop.systemd1.Manager.UnitNew":
				if len(signal.Body) >= 2 {
					unitName := signal.Body[0].(string)
					log.Printf("   [dbus] New unit: %s", unitName)
					w.eventChan <- Event{
						Type:      EventUnitStateChange,
						Source:    "dbus",
						Unit:      unitName,
						Timestamp: time.Now(),
						Data: map[string]string{
							"signal": "UnitNew",
						},
					}
				}

			case "org.freedesktop.systemd1.Manager.UnitRemoved":
				if len(signal.Body) >= 2 {
					unitName := signal.Body[0].(string)
					log.Printf("   [dbus] Unit removed: %s", unitName)
					w.eventChan <- Event{
						Type:      EventUnitStateChange,
						Source:    "dbus",
						Unit:      unitName,
						Timestamp: time.Now(),
						Data: map[string]string{
							"signal": "UnitRemoved",
						},
					}
				}

			case "org.freedesktop.systemd1.Manager.JobNew":
				// Job created (service starting/stopping)
				if len(signal.Body) >= 2 {
					jobID := signal.Body[0].(uint32)
					unitName := signal.Body[2].(string)
					log.Printf("   [dbus] Job started for unit: %s (job %d)", unitName, jobID)
				}

			case "org.freedesktop.systemd1.Manager.JobRemoved":
				// Job completed
				if len(signal.Body) >= 4 {
					unitName := signal.Body[2].(string)
					result := signal.Body[3].(string)
					log.Printf("   [dbus] Job completed for unit: %s (result: %s)", unitName, result)

					// Only trigger reconciliation on failed jobs
					if result != "done" {
						w.eventChan <- Event{
							Type:      EventUnitStateChange,
							Source:    "dbus",
							Unit:      unitName,
							Timestamp: time.Now(),
							Data: map[string]string{
								"signal": "JobRemoved",
								"result": result,
							},
						}
					}
				}
			}

		case <-w.ctx.Done():
			log.Println("   [dbus] Watcher stopped")
			return
		}
	}
}
