package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/store"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func seedRunningTurn(t *testing.T, dir string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if _, err := st.CreateProject(ctx, store.Project{Name: "reef", Path: "/tmp/reef"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateChannel(ctx, store.Channel{ProjectID: 1, Name: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateTurn(ctx, store.Turn{ChannelID: 1, Role: "developer", Backend: "claude"}); err != nil {
		t.Fatal(err)
	}
}

func turnStatus(t *testing.T, dir string) string {
	t.Helper()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	turn, err := st.Turn(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return turn.Status
}

func TestRun_ServeFailsRunningTurn(t *testing.T) {
	dir := t.TempDir()
	seedRunningTurn(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var logs syncBuffer
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"serve", "--addr", "127.0.0.1:0", "--data", dir}, &logs)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "listening on") {
		select {
		case err := <-done:
			t.Fatalf("run returned before listening: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("not listening after 5s; log: %s", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run = %v, want nil", err)
	}

	if got := turnStatus(t, dir); got != "failed" {
		t.Errorf("turn status = %q, want failed", got)
	}
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	msgs, err := st.Messages(context.Background(), 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	const want = "Turn 1 by developer failed: server restarted. The worktree may be dirty; retry or roll back."
	if len(msgs) != 1 || msgs[0].Author != "reef" || msgs[0].Body != want {
		t.Errorf("messages = %+v, want one from reef: %q", msgs, want)
	}
}

func TestRun_SecondServeLeavesRunningTurn(t *testing.T) {
	dir := t.TempDir()
	seedRunningTurn(t, dir)
	held, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	err = run(context.Background(),
		[]string{"serve", "--addr", "127.0.0.1:0", "--data", dir}, &syncBuffer{})
	if err == nil || !strings.Contains(err.Error(), "is in use by another reef") {
		t.Errorf("run = %v, want an in-use error", err)
	}
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}

	if got := turnStatus(t, dir); got != "running" {
		t.Errorf("turn status = %q, want running", got)
	}
}

func TestRun_RefusesNonLoopbackAddr(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:7700", "10.0.0.5:7700", ":7700"} {
		t.Run(addr, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "data")
			err := run(context.Background(),
				[]string{"serve", "--addr", addr, "--data", dir}, &syncBuffer{})
			if err == nil || !strings.Contains(err.Error(), "is not a loopback address") ||
				!strings.Contains(err.Error(), addr) {
				t.Errorf("run = %v, want a loopback error naming %q", err, addr)
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("data dir stat = %v, want not exist", err)
			}
		})
	}
}

func TestRequireLoopback(t *testing.T) {
	tests := []struct {
		addr    string
		wantErr bool
	}{
		{"127.0.0.1:0", false},
		{"localhost:0", false},
		{"[::1]:0", false},
		{"[::ffff:10.0.0.5]:7700", true},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			if err := requireLoopback(tt.addr); (err != nil) != tt.wantErr {
				t.Errorf("requireLoopback(%q) = %v, wantErr %v", tt.addr, err, tt.wantErr)
			}
		})
	}
}
