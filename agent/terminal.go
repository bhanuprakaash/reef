package agent

import (
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

var _escapes = regexp.MustCompile("\x1b\\][^\x07\x1b]*(\x07|\x1b\\\\)|\x1b\\[[0-9;?]*[A-Za-z]|\r|\x04")

// InTerminal is bin with args under a pseudo-terminal made by script(1), for a backend command
// that refuses to run without one, such as `claude mcp login`. script ends with the command's
// exit status. Keep its stdin open: script ends the command when stdin reaches EOF.
func InTerminal(ctx context.Context, bin string, args ...string) *exec.Cmd {
	if runtime.GOOS == "linux" {
		quoted := make([]string, 0, len(args)+1)
		for _, a := range append([]string{bin}, args...) {
			quoted = append(quoted, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
		}
		return exec.CommandContext(ctx, "script", "-qfec", strings.Join(quoted, " "), "/dev/null")
	}
	return exec.CommandContext(ctx, "script", append([]string{"-q", "/dev/null", bin}, args...)...)
}

// Plain is terminal output as text: colours, cursor moves, links and carriage returns removed.
func Plain(b []byte) string {
	return strings.TrimSpace(_escapes.ReplaceAllString(string(b), ""))
}

// RunTerminal runs cmd from InTerminal with its stdin held open and returns its output as Plain text.
func RunTerminal(cmd *exec.Cmd) (string, error) {
	var out Tail
	cmd.Stdout, cmd.Stderr = &out, &out
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	defer stdin.Close()
	err = cmd.Run()
	return Plain(out.Bytes()), err
}
