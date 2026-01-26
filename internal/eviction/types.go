package eviction

// FileMetadata holds information about a cached file.
type FileMetadata struct {
	Path string
	Size int64
}

// Victim represents a file to be evicted.
type Victim struct {
	Key  string
	Size int64
}

// Strategy defines the interface for eviction strategies.
type Strategy interface {
	// OnAdd is called when a new file is added to the cache.
	// It returns the change in total size managed by the strategy (e.g., if key is new, returns size; if updated, returns diff).
	OnAdd(key string, size int64) int64

	// OnAccess is called when a file is accessed.
	OnAccess(key string)

	// GetVictims returns a list of victims to evict to reduce the current size
	// to the target size.
	GetVictims(currentSize int64, targetSize int64) []Victim

	// Remove removes a key from the strategy (e.g. if it was deleted externally).
	Remove(key string)
}
