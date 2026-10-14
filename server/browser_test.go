package server

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCallBrowser_ReplyError(t *testing.T) {
	dir := t.TempDir()
	fakeBrowserSocket(t, dir, `{"error":"boom"}`+"\n")

	_, err := callBrowser(context.Background(), dir, "console", nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if err.Error() != "boom" {
		t.Fatalf("got %q, want %q", err.Error(), "boom")
	}
}

func TestCallBrowser_NoSocket(t *testing.T) {
	dir := t.TempDir()

	_, err := callBrowser(context.Background(), dir, "navigate", map[string]string{"url": "http://localhost"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if err.Error() != _browserUnavailable {
		t.Fatalf("got %q, want %q", err.Error(), _browserUnavailable)
	}
}

func TestCallBrowser_StaleSocket(t *testing.T) {
	dir := t.TempDir()
	l, err := net.Listen("unix", dir+"/"+_browserSocketFile)
	if err != nil {
		t.Fatal(err)
	}
	// A crashed app leaves the socket file behind; a clean Close would
	// unlink it, so disable that to reproduce a stale file.
	if ul, ok := l.(*net.UnixListener); ok {
		ul.SetUnlinkOnClose(false)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = callBrowser(context.Background(), dir, "navigate", map[string]string{"url": "http://localhost"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if err.Error() != _browserUnavailable {
		t.Fatalf("got %q, want %q", err.Error(), _browserUnavailable)
	}
}

func TestCallBrowser_DidNotAnswer(t *testing.T) {
	dir := t.TempDir()
	l, err := net.Listen("unix", dir+"/"+_browserSocketFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		time.Sleep(200 * time.Millisecond) // outlives the test's short ctx deadline
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = callBrowser(ctx, dir, "navigate", map[string]string{"url": "http://localhost"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "browser did not answer:") {
		t.Fatalf("got %q, want it to start with %q", err.Error(), "browser did not answer:")
	}
}
