package eviction

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// Manager manages cache eviction.
type Manager struct {
	cacheDir     string
	monitor      CapacityMonitor
	strategy     Strategy
	currentBytes atomic.Int64
	interval     time.Duration
}

// NewManager creates a new EvictionManager.
func NewManager(cacheDir string, monitor CapacityMonitor, interval time.Duration, strategy Strategy) *Manager {
	return &Manager{
		cacheDir: cacheDir,
		monitor:  monitor,
		interval: interval,
		strategy: strategy,
	}
}

// LoadInitialState scans the cache directory and populates the strategy.
func (m *Manager) LoadInitialState() error {
	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read cache dir: %w", err)
	}

	var totalSize int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			slog.Warn("Failed to get file info", "file", entry.Name(), "error", err)
			continue
		}
		size := info.Size()
		totalSize += size
		m.strategy.OnAdd(entry.Name(), size)
	}

	m.currentBytes.Store(totalSize)
	slog.Info("Initial cache state loaded", "count", len(entries), "size", totalSize)
	return nil
}

// Start runs the background eviction loop.
func (m *Manager) Start(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.RunEviction()
		}
	}
}

// Add adds a file to the strategy and updates size.
func (m *Manager) Add(key string, size int64) {
	diff := m.strategy.OnAdd(key, size)
	m.currentBytes.Add(diff)
}

// Touch updates the access time in the strategy.
func (m *Manager) Touch(key string) {
	m.strategy.OnAccess(key)
}

// RunEviction checks the size and evicts files if needed.
func (m *Manager) RunEviction() {
	current := m.currentBytes.Load()
	toFree, err := m.monitor.BytesToFree(current)
	if err != nil {
		slog.Error("Failed to check capacity", "error", err)
		return
	}

	if toFree <= 0 {
		return
	}

	targetSize := current - toFree
	// Ensure target is not negative (though Strategy logic should handle it)
	if targetSize < 0 {
		targetSize = 0
	}

	victims := m.strategy.GetVictims(current, targetSize)
	if len(victims) == 0 {
		return
	}

	slog.Info("Evicting files", "count", len(victims), "current_size", current, "to_free", toFree, "target", targetSize)

	for _, victim := range victims {
		path := filepath.Join(m.cacheDir, victim.Key)
		err := os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			slog.Error("Failed to remove file", "path", path, "error", err)
			// Continue to next victim?
			// If we can't remove, we shouldn't decrement size?
			// But we remove from strategy to avoid loop.
		}

		m.strategy.Remove(victim.Key)

		// If remove succeeded (or file didn't exist), we consider it gone.
		if err == nil || os.IsNotExist(err) {
			m.currentBytes.Add(-victim.Size)
		}
	}
}
