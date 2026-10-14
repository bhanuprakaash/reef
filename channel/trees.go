package channel

import (
	"context"
	"sync"
)

// Trees hands out one holder at a time per key, a working tree's directory for its write
// turns. It is shared by every Session of one process.
type Trees struct {
	mu   sync.Mutex
	busy map[string]chan struct{}
}

// NewTrees returns Trees with every tree free.
func NewTrees() *Trees {
	return &Trees{busy: map[string]chan struct{}{}}
}

// Lock waits for key and returns the func that frees it. A cancelled ctx ends the wait with
// ctx.Err().
func (t *Trees) Lock(ctx context.Context, key string) (func(), error) {
	t.mu.Lock()
	slot, ok := t.busy[key]
	if !ok {
		slot = make(chan struct{}, 1)
		t.busy[key] = slot
	}
	t.mu.Unlock()
	select {
	case slot <- struct{}{}:
		return func() { <-slot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
