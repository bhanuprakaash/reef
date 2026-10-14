package channel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const _lockPoll = 20 * time.Millisecond

// Trees hands out one holder at a time per key, a working tree's directory for its write
// turns, and counts the readers of a key. Readers do not block holders. The locks are files
// in the data folder, so every process on that folder shares them.
type Trees struct {
	dir string
}

// NewTrees returns Trees with its lock files under dataDir.
func NewTrees(dataDir string) *Trees {
	return &Trees{dir: filepath.Join(dataDir, "locks")}
}

// Read counts a reader of key and returns the func that ends it. It never waits. A reader that
// cannot be recorded, because a rollback is checking the key or the lock file will not open,
// just runs.
func (t *Trees) Read(key string) func() {
	f, err := t.flock(key, ".read", syscall.LOCK_SH|syscall.LOCK_NB)
	if err != nil {
		return func() {}
	}
	return func() { _ = f.Close() }
}

// TryLock is Lock without the wait: it reports false when key is held or has a reader.
func (t *Trees) TryLock(key string) (func(), bool) {
	f, err := t.flock(key, ".lock", syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return nil, false
	}
	r, err := t.flock(key, ".read", syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		return nil, false
	}
	_ = r.Close()
	return func() { _ = f.Close() }, true
}

// Lock waits for key and returns the func that frees it. A cancelled ctx ends the wait with
// ctx.Err().
func (t *Trees) Lock(ctx context.Context, key string) (func(), error) {
	tick := time.NewTicker(_lockPoll)
	defer tick.Stop()
	for {
		f, err := t.flock(key, ".lock", syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}

// flock opens the lock file of key with extension ext and takes it as how says. Closing the
// returned file frees the lock.
func (t *Trees) flock(key, ext string, how int) (*os.File, error) {
	if err := os.MkdirAll(t.dir, 0o755); err != nil {
		return nil, fmt.Errorf("make lock dir: %w", err)
	}
	sum := sha256.Sum256([]byte(key))
	path := filepath.Join(t.dir, hex.EncodeToString(sum[:16])+ext)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return f, nil
}
