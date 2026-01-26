package eviction

import (
	"fmt"
	"sync"
)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]func() Strategy)
)

// Register registers a new eviction strategy factory.
func Register(name string, factory func() Strategy) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
}

// GetStrategy returns a new instance of the strategy with the given name.
func GetStrategy(name string) (Strategy, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("strategy not found: %s", name)
	}
	return factory(), nil
}
