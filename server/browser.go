package server

import (
	"bufio"
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
// and returns its one-line reply. No socket, or nobody listening, is reported
// as the app not being open.
func callBrowser(dataDir, tool string, args any) (browserReply, error) {
	conn, err := net.DialTimeout("unix", filepath.Join(dataDir, _browserSocketFile), _browserTimeout)
	if err != nil {
		return browserReply{}, errors.New(_browserUnavailable)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(_browserTimeout)); err != nil {
		return browserReply{}, fmt.Errorf("set browser socket deadline: %w", err)
	}
	if err := json.NewEncoder(conn).Encode(browserRequest{Tool: tool, Args: args}); err != nil {
		return browserReply{}, errors.New(_browserUnavailable)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return browserReply{}, errors.New(_browserUnavailable)
	}
	var reply browserReply
	if err := json.Unmarshal([]byte(line), &reply); err != nil {
		return browserReply{}, fmt.Errorf("parse browser reply: %w", err)
	}
	return reply, nil
}
