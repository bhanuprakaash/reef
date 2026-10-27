package main

import (
	"crypto/rand"
	"encoding/json"
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

// findSessionDir is the projects dir that already holds sid's session file, searching every
// cwd's folder: a resumed session may have been started under a different cwd (reef moves a
// role's session into its work item's worktree on attach).
func findSessionDir(sid string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sid+".jsonl"))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return filepath.Dir(matches[0]), true
}

// _sessionCounts are the modelUsage fields real claude keeps as session running totals.
var _sessionCounts = []string{
	"inputTokens", "outputTokens", "cacheReadInputTokens", "cacheCreationInputTokens", "costUSD",
}

type sessionTotals struct {
	CostUSD    float64                       `json:"total_cost_usd"`
	ModelUsage map[string]map[string]float64 `json:"modelUsage"`
}

// addSessionTotals turns a replayed result's total_cost_usd and modelUsage into the session's
// running totals, as real claude reports them, also after a --resume: the totals kept in path
// plus this recording's own numbers.
func addSessionTotals(result map[string]any, path string) {
	var tot sessionTotals
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &tot)
	}
	if tot.ModelUsage == nil {
		tot.ModelUsage = map[string]map[string]float64{}
	}
	if c, ok := result["total_cost_usd"].(float64); ok {
		tot.CostUSD += c
		result["total_cost_usd"] = tot.CostUSD
	}
	if usage, ok := result["modelUsage"].(map[string]any); ok {
		for model, v := range usage {
			counts, _ := v.(map[string]any)
			if tot.ModelUsage[model] == nil {
				tot.ModelUsage[model] = map[string]float64{}
			}
			for _, k := range _sessionCounts {
				n, _ := counts[k].(float64)
				tot.ModelUsage[model][k] += n
			}
		}
		for model, counts := range tot.ModelUsage {
			m, _ := usage[model].(map[string]any)
			if m == nil {
				m = map[string]any{}
				usage[model] = m
			}
			for k, n := range counts {
				m[k] = n
			}
		}
	}
	if b, err := json.Marshal(tot); err == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
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
