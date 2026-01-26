package lru

import (
	"container/list"
	"sync"

	"github.com/lucasew/fetchurl/internal/eviction"
)

// LRU implements the eviction.Strategy interface using Least Recently Used logic.
type LRU struct {
	mu    sync.Mutex
	list  *list.List
	items map[string]*list.Element
	sizes map[string]int64
}

type entry struct {
	key  string
	size int64
}

func init() {
	eviction.Register("lru", func() eviction.Strategy {
		return New()
	})
}

func New() *LRU {
	return &LRU{
		list:  list.New(),
		items: make(map[string]*list.Element),
		sizes: make(map[string]int64),
	}
}

func (l *LRU) OnAdd(key string, size int64) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	if elem, ok := l.items[key]; ok {
		l.list.MoveToFront(elem)
		ent := elem.Value.(*entry)
		oldSize := ent.size
		ent.size = size
		l.sizes[key] = size
		return size - oldSize
	}

	ent := &entry{key: key, size: size}
	elem := l.list.PushFront(ent)
	l.items[key] = elem
	l.sizes[key] = size
	return size
}

func (l *LRU) OnAccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if elem, ok := l.items[key]; ok {
		l.list.MoveToFront(elem)
	}
}

func (l *LRU) Remove(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if elem, ok := l.items[key]; ok {
		l.list.Remove(elem)
		delete(l.items, key)
		delete(l.sizes, key)
	}
}

func (l *LRU) GetVictims(currentSize int64, targetSize int64) []eviction.Victim {
	l.mu.Lock()
	defer l.mu.Unlock()

	var victims []eviction.Victim
	size := currentSize

	// Traverse from back without modifying
	elem := l.list.Back()
	for size > targetSize && elem != nil {
		ent := elem.Value.(*entry)
		victims = append(victims, eviction.Victim{Key: ent.key, Size: ent.size})
		size -= ent.size
		elem = elem.Prev()
	}

	return victims
}
