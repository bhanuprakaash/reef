package server

import (
	"net"
	"testing"
)

func TestCallBrowser_ReplyError(t *testing.T) {
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
		_, _ = conn.Write([]byte(`{"error":"boom"}` + "\n"))
	}()

	_, err = callBrowser(dir, "console", nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if err.Error() != "boom" {
		t.Fatalf("got %q, want %q", err.Error(), "boom")
	}
}

func TestCallBrowser_Unavailable(t *testing.T) {
	tests := []struct {
		name string
		give func(t *testing.T) string // returns a data dir
	}{
		{
			name: "no socket",
			give: func(t *testing.T) string {
				return t.TempDir()
			},
		},
		{
			name: "stale socket left by a crash",
			give: func(t *testing.T) string {
				dir := t.TempDir()
				l, err := net.Listen("unix", dir+"/"+_browserSocketFile)
				if err != nil {
					t.Fatal(err)
				}
				// A crashed app leaves the socket file behind; a clean Close
				// would unlink it, so disable that to reproduce a stale file.
				if ul, ok := l.(*net.UnixListener); ok {
					ul.SetUnlinkOnClose(false)
				}
				if err := l.Close(); err != nil {
					t.Fatal(err)
				}
				return dir
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := callBrowser(tt.give(t), "navigate", map[string]string{"url": "http://localhost"})
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if err.Error() != _browserUnavailable {
				t.Fatalf("got %q, want %q", err.Error(), _browserUnavailable)
			}
		})
	}
}
