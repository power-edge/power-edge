//go:build !linux
// +build !linux

package watcher

import (
	"log"
)

// Stub implementations for non-Linux platforms
// Event watchers are Linux-specific and use systemd, inotify, auditd, and dbus

func (w *EventWatcher) runInotifyWatcher() {
	defer w.wg.Done()
	log.Println("   [inotify] Not supported on this platform (Linux-only)")
}

func (w *EventWatcher) runJournaldWatcher() {
	defer w.wg.Done()
	log.Println("   [journald] Not supported on this platform (Linux-only)")
}

func (w *EventWatcher) runAuditdWatcher() {
	defer w.wg.Done()
	log.Println("   [auditd] Not supported on this platform (Linux-only)")
}

func (w *EventWatcher) runAuditdViaJournald() {
	log.Println("   [auditd-fallback] Not supported on this platform (Linux-only)")
}

func (w *EventWatcher) runDbusWatcher() {
	defer w.wg.Done()
	log.Println("   [dbus] Not supported on this platform (Linux-only)")
}
