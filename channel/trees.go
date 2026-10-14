package channel

import (
	"context"
	"sync"
)

// Trees hands out one holder at a time per key, a working tree's directory for its write
// turns, and counts the readers of a key. Readers do not block holders. It is shared by every
// Session of one process.
type Trees struct {
	mu      sync.Mutex
	busy    map[string]chan struct{}
	readers map[string]int
}

// NewTrees returns Trees with every tree free.
func NewTrees() *Trees {
	return &Trees{busy: map[string]chan struct{}{}, readers: map[string]int{}}
}

// Read counts a reader of key and returns the func that ends it. It never waits.
func (t *Trees) Read(key string) func() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.readers[key]++
	return func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.readers[key]--
	}
}

// TryLock is Lock without the wait: it reports false when key is held or has a reader.
func (t *Trees) TryLock(key string) (func(), bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.readers[key] > 0 {
		return nil, false
	}
	slot := t.slotLocked(key)
	select {
	case slot <- struct{}{}:
		return func() { <-slot }, true
	default:
		return nil, false
	}
}

// Lock waits for key and returns the func that frees it. A cancelled ctx ends the wait with
// ctx.Err().
func (t *Trees) Lock(ctx context.Context, key string) (func(), error) {
	t.mu.Lock()
	slot := t.slotLocked(key)
	t.mu.Unlock()
	select {
	case slot <- struct{}{}:
		return func() { <-slot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *Trees) slotLocked(key string) chan struct{} {
	slot, ok := t.busy[key]
	if !ok {
		slot = make(chan struct{}, 1)
		t.busy[key] = slot
	}
	return slot
}
