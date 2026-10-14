package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runToolUse applies the side effect of one recorded tool call in cwd, so the files and
// git history a recipe checks after a replayed turn are the ones the real turn made. mcpConfig
// is the current run's --mcp-config file, read for an mcp__<server>__<tool> call.
func runToolUse(name string, input json.RawMessage, cwd, mcpConfig string) {
	switch {
	case name == "Write":
		var in struct {
			FilePath string `json:"file_path"`
			Content  string `json:"content"`
		}
		if json.Unmarshal(input, &in) != nil {
			return
		}
		_ = os.MkdirAll(filepath.Dir(in.FilePath), 0o755)
		_ = os.WriteFile(in.FilePath, []byte(in.Content), 0o644)
	case name == "Edit":
		var in struct {
			FilePath   string `json:"file_path"`
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		}
		if json.Unmarshal(input, &in) != nil {
			return
		}
		applyEdit(in.FilePath, in.OldString, in.NewString, in.ReplaceAll)
	case name == "MultiEdit":
		var in struct {
			FilePath string `json:"file_path"`
			Edits    []struct {
				OldString  string `json:"old_string"`
				NewString  string `json:"new_string"`
				ReplaceAll bool   `json:"replace_all"`
			} `json:"edits"`
		}
		if json.Unmarshal(input, &in) != nil {
			return
		}
		for _, e := range in.Edits {
			applyEdit(in.FilePath, e.OldString, e.NewString, e.ReplaceAll)
		}
	case name == "Bash":
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &in) != nil {
			return
		}
		cmd := exec.Command("sh", "-c", in.Command)
		cmd.Dir = cwd
		cmd.Env = os.Environ()
		_ = cmd.Run()
	case strings.HasPrefix(name, "mcp__"):
		parts := strings.SplitN(name, "__", 3)
		if len(parts) == 3 {
			callMCPTool(mcpConfig, parts[1], parts[2], input)
		}
	}
}

func applyEdit(path, oldStr, newStr string, replaceAll bool) {
	if oldStr == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	n := 1
	if replaceAll {
		n = -1
	}
	_ = os.WriteFile(path, []byte(strings.Replace(string(b), oldStr, newStr, n)), 0o644)
}
