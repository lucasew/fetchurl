package eviction

import (
	"fmt"
	"log/slog"
	"syscall"
)

// CapacityMonitor defines the interface for checking if eviction is needed.
type CapacityMonitor interface {
	// BytesToFree returns the number of bytes that should be evicted.
	// Returns 0 if no eviction is needed.
	BytesToFree(currentSize int64) (int64, error)
}

// MaxCacheSizeMonitor triggers eviction when cache exceeds a fixed size.
type MaxCacheSizeMonitor struct {
	MaxBytes int64
}

func (m *MaxCacheSizeMonitor) BytesToFree(currentSize int64) (int64, error) {
	if currentSize > m.MaxBytes {
		return currentSize - m.MaxBytes, nil
	}
	return 0, nil
}

// MinFreeSpaceMonitor triggers eviction when disk free space is below a threshold.
type MinFreeSpaceMonitor struct {
	Path        string
	MinFreeBytes int64
}

func (m *MinFreeSpaceMonitor) BytesToFree(currentSize int64) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(m.Path, &stat); err != nil {
		return 0, fmt.Errorf("failed to check disk space: %w", err)
	}

	// Available blocks * block size
	freeSpace := int64(stat.Bavail) * int64(stat.Bsize)

	slog.Debug("Disk space check", "path", m.Path, "free_bytes", freeSpace, "min_required", m.MinFreeBytes)

	if freeSpace < m.MinFreeBytes {
		needed := m.MinFreeBytes - freeSpace
		// Ensure we don't try to free more than we have?
		// If needed > currentSize, we just return needed, and the manager handles up to currentSize.
		return needed, nil
	}
	return 0, nil
}
