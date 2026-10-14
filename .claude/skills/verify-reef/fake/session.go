package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var _nonSessionChar = regexp.MustCompile(`[^A-Za-z0-9]`)

// sessionKey is the folder name claude keeps a cwd's sessions under, the same mapping
// verify-reef's cleanup uses.
func sessionKey(cwd string) string {
	return _nonSessionChar.ReplaceAllString(cwd, "-")
}

// projectsDir is ~/.claude/projects/<key> for cwd, made if missing.
func projectsDir(cwd string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".claude", "projects", sessionKey(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// newUUID is a random v4 UUID, the same shape real claude uses for a session id.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func roleFile(dir, sid string) string { return filepath.Join(dir, sid+".role") }

func readRole(dir, sid string) string {
	b, err := os.ReadFile(roleFile(dir, sid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeRole(dir, sid, role string) {
	_ = os.WriteFile(roleFile(dir, sid), []byte(role), 0o644)
}

func sessionFilePath(dir, sid string) string { return filepath.Join(dir, sid+".jsonl") }

func sessionFileExists(dir, sid string) bool {
	_, err := os.Stat(sessionFilePath(dir, sid))
	return err == nil
}

// openFakeLog opens REEF_FAKE_LOG for append, or a discarded sink when it is unset.
func openFakeLog() *os.File {
	path := os.Getenv("REEF_FAKE_LOG")
	if path == "" {
		f, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		return f
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		f, _ = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	}
	return f
}
