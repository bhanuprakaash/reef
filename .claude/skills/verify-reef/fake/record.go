package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// runRecord is REEF_VERIFY_CLAUDE=record: run the real claude, forward stdin and stdout
// unchanged, and save each turn whose prompt matches a scenario as that scenario's
// recording. A turn ends at its "result" line, since reef sends one prompt at a time and
// waits for the turn to end before the next.
func runRecord(args []string) error {
	real := findRealClaude()
	if real == "" {
		return errors.New("no real claude found on PATH outside the fake's own directory")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	dir, err := projectsDir(cwd)
	if err != nil {
		return err
	}
	cmd := exec.Command(real, args...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	fakeLog := openFakeLog()
	defer fakeLog.Close()
	stdoutR := bufio.NewReader(stdoutPipe)
	stdinR := bufio.NewReader(os.Stdin)
	// A wake that resumes an earlier session sends no preamble, so the role cannot be read
	// off this prompt; it was saved under the session id on that session's first turn.
	cli := parseArgs(args)
	role := readRole(dir, cli.resume)

	for {
		promptLine, rerr := stdinR.ReadBytes('\n')
		if len(promptLine) > 0 {
			if _, werr := stdin.Write(promptLine); werr != nil {
				break
			}
			prompt := extractPrompt(promptLine)
			if prompt != "" && role == "" {
				if m := _roleRe.FindStringSubmatch(prompt); m != nil {
					role = m[1]
				}
			}
			name := ""
			if prompt != "" {
				name = matchScenario(role, textPart(prompt))
			}
			recordTurn(stdoutR, name, role, prompt, args, fakeLog, dir)
		}
		if rerr != nil {
			break
		}
	}
	_ = stdin.Close()
	return cmd.Wait()
}

// recordTurn copies claude's stdout for one turn to our own stdout, and, when name is set,
// saves the recording. projDir is where this session's role is saved, under its session id,
// for a later turn that resumes it with no preamble to read role back.
func recordTurn(
	stdoutR *bufio.Reader, name, role, prompt string, args []string, fakeLog *os.File, projDir string,
) {
	var lines []recLine
	t0 := time.Now()
	// finish saves the recording and logs before forwarding final (the line that ends the
	// turn, or nil at EOF with none): reef-agent kills the fake's whole process group right
	// after it reads that line off our stdout, so bookkeeping after the forward races losing.
	finish := func(final []byte) {
		if role != "" {
			if sid := sessionIDFromLines(lines); sid != "" {
				writeRole(projDir, sid, role)
			}
		}
		if name == "" || prompt == "" {
			fakeLog.WriteString(time.Now().Format(time.RFC3339) + " record: not recorded (role=" + orNone(role) + ")\n")
		} else if err := writeRecording(name, role, prompt, args, lines); err != nil {
			fakeLog.WriteString(time.Now().Format(time.RFC3339) + " record: " + name + " failed: " + err.Error() + "\n")
		} else {
			fakeLog.WriteString(time.Now().Format(time.RFC3339) + " record: recorded " + name + "\n")
		}
		if final != nil {
			os.Stdout.Write(final)
		}
	}
	for {
		out, oerr := stdoutR.ReadBytes('\n')
		if len(out) > 0 {
			lines = append(lines, recLine{T: time.Since(t0).Seconds(), Out: strings.TrimRight(string(out), "\n")})
			var kind struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(out, &kind)
			if kind.Type == "result" {
				finish(out)
				return
			}
			os.Stdout.Write(out)
		}
		if oerr != nil {
			break
		}
	}
	finish(nil)
}

var _sessionIDRe = regexp.MustCompile(`"session_id":"([^"]+)"`)

// sessionIDFromLines is the first session id found in lines, claude's stdout of one turn.
func sessionIDFromLines(lines []recLine) string {
	for _, l := range lines {
		if m := _sessionIDRe.FindStringSubmatch(l.Out); m != nil {
			return m[1]
		}
	}
	return ""
}

// writeRecording saves name's recording: a header line (cwd, the session id found in
// lines, role, prompt, args), then lines as recorded. The whole file is built in memory
// and written with one os.WriteFile: reef-agent kills the fake's whole process group right
// after its last line ends the turn, so a loop of small writes here races losing the end
// of the recording.
func writeRecording(name, role, prompt string, args []string, lines []recLine) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	session := sessionIDFromLines(lines)
	dir := recordingsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	hdr := recHeader{Cwd: cwd, Session: session, Role: role, Prompt: prompt, Args: args}
	buf.Write(marshalLine(hdr))
	buf.WriteByte('\n')
	for _, l := range lines {
		l.Out = trimInit(l.Out)
		buf.Write(marshalLine(l))
		buf.WriteByte('\n')
	}

	// os.WriteFile truncates the destination before writing it, so a kill between those
	// two steps would leave an empty recording; write the temp file next door instead and
	// rename it in, one atomic step that either lands whole or not at all.
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(buf.Bytes())
	cerr := tmp.Close()
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return werr
	}
	if cerr != nil {
		_ = os.Remove(tmp.Name())
		return cerr
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name+".jsonl"))
}

// trimInit keeps only the fields of a system init line that the adapter reads: the rest
// lists the recording machine's skills, plugins, agents and home paths, and the repo is public.
func trimInit(out string) string {
	var l map[string]any
	if json.Unmarshal([]byte(out), &l) != nil || l["type"] != "system" || l["subtype"] != "init" {
		return out
	}
	kept := map[string]any{}
	for _, k := range []string{"type", "subtype", "cwd", "session_id", "model"} {
		if v, ok := l[k]; ok {
			kept[k] = v
		}
	}
	return string(marshalLine(kept))
}

// findRealClaude is the first "claude" on PATH that is not in the fake's own directory.
func findRealClaude() string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	selfDir, err := filepath.Abs(filepath.Dir(self))
	if err != nil {
		return ""
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		abs, err := filepath.Abs(dir)
		if err != nil || abs == selfDir {
			continue
		}
		p := filepath.Join(dir, "claude")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}
