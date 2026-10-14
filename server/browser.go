package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"time"
)

const (
	_browserSocketFile  = "browser.sock"
	_browserTimeout     = 10 * time.Second
	_browserUnavailable = "browser unavailable: the Reef app is not open"
)

type browserRequest struct {
	Tool string `json:"tool"`
	Args any    `json:"args,omitempty"`
}

type browserReply struct {
	Text  string `json:"text,omitempty"`
	Image string `json:"image,omitempty"`
	Error string `json:"error,omitempty"`
}

// callBrowser sends one tool call to the Reef app's browser socket in dataDir
// and returns its one-line reply. Only a failed dial is reported as the app
// not being open; once connected, a write, read or deadline failure means the
// app is there but did not answer.
func callBrowser(ctx context.Context, dataDir, tool string, args any) (browserReply, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", filepath.Join(dataDir, _browserSocketFile))
	if err != nil {
		return browserReply{}, errors.New(_browserUnavailable)
	}
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(_browserTimeout)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return browserReply{}, fmt.Errorf("browser did not answer: %w", err)
	}
	if err := json.NewEncoder(conn).Encode(browserRequest{Tool: tool, Args: args}); err != nil {
		return browserReply{}, fmt.Errorf("browser did not answer: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return browserReply{}, fmt.Errorf("browser did not answer: %w", err)
	}
	var reply browserReply
	if err := json.Unmarshal([]byte(line), &reply); err != nil {
		return browserReply{}, fmt.Errorf("parse browser reply: %w", err)
	}
	if reply.Error != "" {
		return browserReply{}, errors.New(reply.Error)
	}
	return reply, nil
}
