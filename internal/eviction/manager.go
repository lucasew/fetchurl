package eviction

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/lucasew/fetchurl/internal/eviction/policy"
)

// Manager manages cache eviction.
type Manager struct {
	store        Store
	policies     []policy.Policy
	strategy     Strategy
	currentBytes atomic.Int64
	interval     time.Duration
}

// NewManager creates a new EvictionManager.
func NewManager(policies []policy.Policy, interval time.Duration, strategy Strategy) *Manager {
	return &Manager{
		policies: policies,
		interval: interval,
		strategy: strategy,
	}
}

// SetStore sets the underlying storage for the manager.
func (m *Manager) SetStore(store Store) {
	m.store = store
}

// LoadInitialState scans the cache directory and populates the strategy.
func (m *Manager) LoadInitialState() error {
	if m.store == nil {
		return fmt.Errorf("store not initialized")
	}

	var totalSize int64
	var count int

	err := m.store.Walk(func(key string, size int64) error {
		totalSize += size
		count++
		m.strategy.OnAdd(key, size)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk cache: %w", err)
	}

	m.currentBytes.Store(totalSize)
	slog.Info("Initial cache state loaded", "count", count, "size", totalSize)
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
	if m.store == nil {
		slog.Error("Store not initialized")
		return
	}

	current := m.currentBytes.Load()
	var maxToFree int64

	for _, p := range m.policies {
		toFree, err := p.BytesToFree(current)
		if err != nil {
			slog.Error("Failed to check capacity policy", "error", err)
			continue
		}
		if toFree > maxToFree {
			maxToFree = toFree
		}
	}

	if maxToFree <= 0 {
		return
	}

	targetSize := current - maxToFree
	// Ensure target is not negative (though Strategy logic should handle it)
	if targetSize < 0 {
		targetSize = 0
	}

	victims := m.strategy.GetVictims(current, targetSize)
	if len(victims) == 0 {
		return
	}

	slog.Info("Evicting files", "count", len(victims), "current_size", current, "to_free", maxToFree, "target", targetSize)

	for _, victim := range victims {
		err := m.store.Delete(victim.Key)

		if err != nil {
			// We can't check os.IsNotExist here easily without importing os.
			// Ideally Store.Delete should be idempotent.
			slog.Error("Failed to remove file", "key", victim.Key, "error", err)
		}

		m.strategy.Remove(victim.Key)

		// We assume it's gone.
		m.currentBytes.Add(-victim.Size)
	}
}
