package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runFake is REEF_VERIFY_CLAUDE=fake (the default): one turn per stdin line, replayed from
// a recording picked by role and prompt, to EOF.
func runFake(args []string) error {
	cli := parseArgs(args)
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	dir, err := projectsDir(cwd)
	if err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	fakeLog := openFakeLog()
	defer fakeLog.Close()

	var sid, role string
	if cli.resume != "" {
		sid = cli.resume
		if !sessionFileExists(dir, sid) {
			// Real claude resumes a session id from any cwd, not only the one it was
			// created in (reef moves a role's session into its feature's worktree on
			// promote): find wherever this session actually lives.
			if found, ok := findSessionDir(sid); ok {
				dir = found
			} else {
				writeLine(out, map[string]any{
					"type": "result", "is_error": true,
					"errors": []string{"No conversation found with session ID: " + sid},
				})
				out.Flush()
				os.Exit(1)
			}
		}
		role = readRole(dir, sid)
	} else {
		sid = newUUID()
	}
	writeLine(out, map[string]any{"type": "system", "subtype": "init", "session_id": sid})
	out.Flush()

	sessf, err := os.OpenFile(sessionFilePath(dir, sid), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer sessf.Close()

	in := bufio.NewReader(os.Stdin)
	for {
		line, rerr := in.ReadBytes('\n')
		if len(line) > 0 {
			handleTurn(line, cwd, sid, &role, dir, sessf, out, fakeLog, cli.mcpConfig)
			out.Flush()
		}
		if rerr != nil {
			break
		}
	}
	return nil
}

func writeLine(out *bufio.Writer, v any) {
	out.Write(marshalLine(v))
	out.WriteByte('\n')
}

func extractPrompt(raw []byte) string {
	var u struct {
		Type    string `json:"type"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &u) != nil || u.Type != "user" {
		return ""
	}
	return u.Message.Content
}

func appendSessionEvent(f *os.File, v map[string]any, cwd, sid string) {
	v["cwd"] = cwd
	v["sessionId"] = sid
	f.Write(marshalLine(v))
	f.Write([]byte("\n"))
}

func appendSessionRaw(f *os.File, raw, cwd, sid string) {
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return
	}
	appendSessionEvent(f, m, cwd, sid)
}

func handleTurn(
	raw []byte, cwd, sid string, role *string, dir string, sessf *os.File, out *bufio.Writer,
	fakeLog *os.File, mcpConfig string,
) {
	prompt := extractPrompt(raw)
	if prompt == "" {
		return
	}
	if *role == "" {
		if m := _roleRe.FindStringSubmatch(prompt); m != nil {
			*role = m[1]
			writeRole(dir, sid, *role)
		}
	}
	appendSessionEvent(sessf, map[string]any{
		"type": "user", "message": map[string]any{"role": "user", "content": prompt},
	}, cwd, sid)

	name := matchScenario(*role, textPart(prompt))
	if name == "" {
		msg := fmt.Sprintf("no scenario for role %s: %s", orNone(*role), truncate(textPart(prompt), 200))
		fmt.Fprintln(os.Stderr, "fake claude:", msg)
		writeLine(out, map[string]any{"type": "result", "is_error": true, "result": msg})
		logTurn(fakeLog, *role, "none", cwd, sid)
		return
	}
	replay(name, *role, cwd, sid, prompt, out, sessf, fakeLog, mcpConfig)
}

type recHeader struct {
	Cwd     string   `json:"cwd"`
	Session string   `json:"session"`
	Role    string   `json:"role"`
	Prompt  string   `json:"prompt"`
	Args    []string `json:"args"`
}

// recLine is one recorded stdout line: Out is the raw text claude printed, T the seconds
// since the prompt that produced it was sent.
type recLine struct {
	T   float64 `json:"t"`
	Out string  `json:"out"`
}

// replay prints name's recording for this turn, cwd and session swapped in, sleeping the
// recorded gaps (capped at 1s), running the side effects of its tool calls and appending
// the session file the way real claude would.
func replay(
	name, role, cwd, sid, prompt string, out *bufio.Writer, sessf *os.File, fakeLog *os.File,
	mcpConfig string,
) {
	path := filepath.Join(recordingsDir(), name+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		msg := fmt.Sprintf("scenario %s has no recording: %v", name, err)
		fmt.Fprintln(os.Stderr, "fake claude:", msg)
		writeLine(out, map[string]any{"type": "result", "is_error": true, "result": msg})
		logTurn(fakeLog, role, "none", cwd, sid)
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	if !sc.Scan() {
		logTurn(fakeLog, role, "none", cwd, sid)
		return
	}
	var hdr recHeader
	_ = json.Unmarshal(sc.Bytes(), &hdr)

	var lines []recLine
	for sc.Scan() {
		var l recLine
		if json.Unmarshal(sc.Bytes(), &l) == nil {
			lines = append(lines, l)
		}
	}
	// Logged now, not after the last line: reef-agent kills the fake's whole process group
	// right after reading the turn's last line, which races losing a log line written after.
	logTurn(fakeLog, role, name, cwd, sid)

	errByID := toolResultErrors(lines)
	prev := 0.0
	for _, l := range lines {
		gap := l.T - prev
		prev = l.T
		if gap > 1 {
			gap = 1
		}
		if gap > 0 {
			time.Sleep(time.Duration(gap * float64(time.Second)))
		}

		replaced := strings.ReplaceAll(l.Out, hdr.Cwd, cwd)
		replaced = strings.ReplaceAll(replaced, hdr.Session, sid)

		isReplayPrompt := false
		var generic map[string]any
		if json.Unmarshal([]byte(replaced), &generic) == nil {
			if t, _ := generic["type"].(string); t == "user" {
				if ir, _ := generic["isReplay"].(bool); ir {
					isReplayPrompt = true
					if msg, ok := generic["message"].(map[string]any); ok {
						msg["content"] = prompt
					}
					replaced = string(marshalLine(generic))
				}
			}
		}

		fmt.Fprintln(out, replaced)
		out.Flush()

		var kind struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(replaced), &kind)
		switch {
		case kind.Type == "assistant":
			appendSessionRaw(sessf, replaced, cwd, sid)
			applyToolUses(replaced, cwd, mcpConfig, errByID)
		case kind.Type == "user" && !isReplayPrompt:
			appendSessionRaw(sessf, replaced, cwd, sid)
		}
	}
}

// toolResultErrors maps a recording's tool_use ids to whether their recorded tool_result
// was an error.
func toolResultErrors(lines []recLine) map[string]bool {
	errByID := map[string]bool{}
	for _, l := range lines {
		var o struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type      string `json:"type"`
					ToolUseID string `json:"tool_use_id"`
					IsError   bool   `json:"is_error"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(l.Out), &o) != nil || o.Type != "user" {
			continue
		}
		for _, b := range o.Message.Content {
			if b.Type == "tool_result" {
				errByID[b.ToolUseID] = b.IsError
			}
		}
	}
	return errByID
}

// applyToolUses runs the side effect of every tool_use block of an assistant line whose
// recorded result was not an error.
func applyToolUses(raw, cwd, mcpConfig string, errByID map[string]bool) {
	var a struct {
		Message struct {
			Content []struct {
				Type  string          `json:"type"`
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(raw), &a) != nil {
		return
	}
	for _, b := range a.Message.Content {
		if b.Type != "tool_use" || errByID[b.ID] {
			continue
		}
		runToolUse(b.Name, b.Input, cwd, mcpConfig)
	}
}

func recordingsDir() string {
	if d := os.Getenv("REEF_FAKE_RECORDINGS"); d != "" {
		return d
	}
	return "recordings"
}

func logTurn(fakeLog *os.File, role, scenario, cwd, sid string) {
	fmt.Fprintf(fakeLog, "%s pid=%d mode=fake role=%s scenario=%s cwd=%s session=%s\n",
		time.Now().Format(time.RFC3339), os.Getpid(), orNone(role), scenario, cwd, sid)
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
