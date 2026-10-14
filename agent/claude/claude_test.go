package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		give string
		want []agent.Event
	}{
		{
			name: "init",
			give: `{"type":"system","subtype":"init","cwd":"/private/tmp/scratch-claude","session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381","model":"claude-opus-5-5","permissionMode":"bypassPermissions"}`,
			want: []agent.Event{{Kind: agent.KindSession, SessionID: "3c1296cc-1e28-4167-a8a8-dfb0facf9381"}},
		},
		{
			name: "hook is skipped",
			give: `{"type":"system","subtype":"hook_started","hook_id":"015c16a7-c42b-4557-b43a-d55ccfb33e5a","hook_name":"SessionStart:startup","hook_event":"SessionStart","session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381"}`,
		},
		{
			name: "rate limit is skipped",
			give: `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"},"session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381"}`,
		},
		{
			name: "text delta",
			give: `{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"` + "`ls" + `"}},"session_id":"9fc87746-5f3e-4a0d-8a30-540ee65a50c9","parent_tool_use_id":null}`,
			want: []agent.Event{{Kind: agent.KindText, Text: "`ls"}},
		},
		{
			name: "tool input delta is skipped",
			give: `{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\": \"ls -la"}},"session_id":"9fc87746-5f3e-4a0d-8a30-540ee65a50c9","parent_tool_use_id":null}`,
		},
		{
			name: "tool call is held until its message's usage",
			give: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01QXVfbj1JZFm29Aqmf9Gb8z","name":"Bash","input":{"command":"echo hello-from-tool","description":"Print test string"},"caller":{"type":"direct"}}]},"parent_tool_use_id":null,"session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381"}`,
		},
		{
			name: "assistant text is skipped",
			give: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Output: ` + "`hello-from-tool`" + `"}]},"parent_tool_use_id":null,"session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381"}`,
		},
		{
			name: "tool result",
			give: `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01QXVfbj1JZFm29Aqmf9Gb8z","type":"tool_result","content":"hello-from-tool","is_error":false}]},"parent_tool_use_id":null,"session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381"}`,
			want: []agent.Event{{Kind: agent.KindToolResult, ToolID: "toolu_01QXVfbj1JZFm29Aqmf9Gb8z", Output: "hello-from-tool"}},
		},
		{
			name: "subagent tool call",
			give: `{"type":"assistant","message":{"model":"claude-sonnet-5-5","role":"assistant","content":[{"type":"tool_use","id":"toolu_01BreQyuMiV9cKxFWNeiHK4Q","name":"Bash","input":{"command":"echo sub-hello","description":"Print sub-hello"},"caller":{"type":"direct"}}]},"parent_tool_use_id":"toolu_01RaXuLo7hgPeWd6uAE8s6vP","agent_id":"acbb09e2905459672","session_id":"5cb3eb98-0d14-468a-9526-3f91b3068676"}`,
			want: []agent.Event{{
				Kind:         agent.KindToolCall,
				ToolID:       "toolu_01BreQyuMiV9cKxFWNeiHK4Q",
				ToolName:     "Bash",
				ParentToolID: "toolu_01RaXuLo7hgPeWd6uAE8s6vP",
				Input:        json.RawMessage(`{"command":"echo sub-hello","description":"Print sub-hello"}`),
			}},
		},
		{
			name: "subagent tool result",
			give: `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01BreQyuMiV9cKxFWNeiHK4Q","type":"tool_result","content":"sub-hello","is_error":false}]},"parent_tool_use_id":"toolu_01RaXuLo7hgPeWd6uAE8s6vP","agent_id":"acbb09e2905459672","session_id":"5cb3eb98-0d14-468a-9526-3f91b3068676"}`,
			want: []agent.Event{{
				Kind:         agent.KindToolResult,
				ToolID:       "toolu_01BreQyuMiV9cKxFWNeiHK4Q",
				Output:       "sub-hello",
				ParentToolID: "toolu_01RaXuLo7hgPeWd6uAE8s6vP",
			}},
		},
		{
			name: "tool result as text blocks",
			give: `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01","type":"tool_result","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}],"is_error":true}]}}`,
			want: []agent.Event{{Kind: agent.KindToolResult, ToolID: "toolu_01", Output: "a\nb", IsError: true}},
		},
		{
			name: "result",
			give: `{"type":"result","subtype":"success","is_error":false,"result":"Output: hello-from-tool","session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381","usage":{"input_tokens":4,"cache_creation_input_tokens":10112,"cache_read_input_tokens":22898,"output_tokens":123}}`,
			want: []agent.Event{
				{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 4, OutputTokens: 123, CacheReadTokens: 22898, CacheWriteTokens: 10112}},
				{Kind: agent.KindTurnEnd},
			},
		},
		{
			name: "error result",
			give: `{"type":"result","subtype":"error_during_execution","is_error":true,"session_id":"00000000-0000-0000-0000-000000000000","usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0},"errors":["No conversation found with session ID: 00000000-0000-0000-0000-000000000000"]}`,
			want: []agent.Event{
				{Kind: agent.KindError, Text: "No conversation found with session ID: 00000000-0000-0000-0000-000000000000"},
			},
		},
		{
			name: "message usage",
			give: `{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":2,"cache_creation_input_tokens":102,"cache_read_input_tokens":18205,"output_tokens":72}},"session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381","parent_tool_use_id":null}`,
			want: []agent.Event{
				{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 2, OutputTokens: 72, CacheReadTokens: 18205, CacheWriteTokens: 102}},
			},
		},
		{
			name: "assistant usage is skipped",
			give: `{"type":"assistant","message":{"id":"msg_011CfzhA1pca1AC5oMzamfLt","role":"assistant","content":[{"type":"text","text":"42"}],"usage":{"input_tokens":2,"cache_creation_input_tokens":425,"cache_read_input_tokens":18362,"output_tokens":3}},"parent_tool_use_id":null}`,
		},
		{
			name: "result of a background task",
			give: `{"type":"result","subtype":"success","is_error":false,"result":"","origin":{"kind":"task-notification"},"num_turns":0,"session_id":"d6c291c5-ec96-403e-9f97-720d1dee78db","usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0}}`,
		},
		{
			name: "not json",
			give: `claude: warming up`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parse([]byte(tt.give), &state{})
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parse() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParse_toolCallsFollowUsage(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01AbYRGNn2oNiJAXmFbwaF6C","name":"Read","input":{"file_path":"/tmp/reef-rec88/note.txt"}},{"type":"tool_use","id":"toolu_02","name":"Bash","input":{"command":"echo second"}}],"usage":{"input_tokens":2,"cache_creation_input_tokens":8139,"cache_read_input_tokens":7817,"output_tokens":16}},"parent_tool_use_id":null}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_sub","name":"Read","input":{}}]},"parent_tool_use_id":"toolu_agent"}`,
		`{"type":"stream_event","event":{"type":"content_block_stop","index":0},"parent_tool_use_id":null}`,
		`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":2,"cache_creation_input_tokens":8139,"cache_read_input_tokens":7817,"output_tokens":63}},"parent_tool_use_id":null}`,
		`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":2,"cache_creation_input_tokens":105,"cache_read_input_tokens":18204,"output_tokens":3}},"parent_tool_use_id":null}`,
	}
	want := []agent.Event{
		{Kind: agent.KindToolCall, ToolID: "toolu_sub", ToolName: "Read", ParentToolID: "toolu_agent", Input: json.RawMessage(`{}`)},
		{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 2, OutputTokens: 63, CacheReadTokens: 7817, CacheWriteTokens: 8139}},
		{Kind: agent.KindToolCall, ToolID: "toolu_01AbYRGNn2oNiJAXmFbwaF6C", ToolName: "Read", Input: json.RawMessage(`{"file_path":"/tmp/reef-rec88/note.txt"}`)},
		{Kind: agent.KindToolCall, ToolID: "toolu_02", ToolName: "Bash", Input: json.RawMessage(`{"command":"echo second"}`)},
		{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 2, OutputTokens: 3, CacheReadTokens: 18204, CacheWriteTokens: 105}},
	}
	var st state
	var got []agent.Event
	for _, line := range lines {
		got = append(got, parse([]byte(line), &st)...)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

func TestParse_toolCallBeforeItsResult(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/tmp/note.txt"}}]},"parent_tool_use_id":null}`,
		`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01","type":"tool_result","content":"note","is_error":false}]},"parent_tool_use_id":null}`,
		`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":2,"cache_creation_input_tokens":8139,"cache_read_input_tokens":7817,"output_tokens":63}},"parent_tool_use_id":null}`,
	}
	want := []agent.Event{
		{Kind: agent.KindToolCall, ToolID: "toolu_01", ToolName: "Read", Input: json.RawMessage(`{"file_path":"/tmp/note.txt"}`)},
		{Kind: agent.KindToolResult, ToolID: "toolu_01", Output: "note"},
		{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 2, OutputTokens: 63, CacheReadTokens: 7817, CacheWriteTokens: 8139}},
	}
	var st state
	var got []agent.Event
	for _, line := range lines {
		got = append(got, parse([]byte(line), &st)...)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

func TestParse_breakBetweenMessages(t *testing.T) {
	lines := []string{
		`{"type":"stream_event","event":{"type":"message_start","message":{"role":"assistant"}},"parent_tool_use_id":null}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"About tides."}},"parent_tool_use_id":null}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" Looking."}},"parent_tool_use_id":null}`,
		`{"type":"stream_event","event":{"type":"message_start","message":{"role":"assistant"}},"parent_tool_use_id":null}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Files:"}},"parent_tool_use_id":null}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" a, b"}},"parent_tool_use_id":null}`,
	}
	var st state
	var got string
	for _, line := range lines {
		for _, ev := range parse([]byte(line), &st) {
			got += ev.Text
		}
	}
	if want := "About tides. Looking.\n\nFiles: a, b"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// background-task.jsonl is a real session, trimmed: a background Bash task starts in turn 1
// and ends while turn 2 runs; claude then runs a turn of its own (init, no echoed prompt,
// result with origin task-notification) before it answers turn 3.
func TestParse_backgroundTaskTurn(t *testing.T) {
	raw, err := os.ReadFile("testdata/background-task.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var st state
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		for _, ev := range parse([]byte(line), &st) {
			s := string(ev.Kind)
			switch ev.Kind {
			case agent.KindText:
				s += " " + ev.Text
			case agent.KindToolCall:
				s += " " + ev.ToolName
			case agent.KindUsage:
				s += fmt.Sprintf(" out=%d", ev.Usage.OutputTokens)
			}
			got = append(got, s)
		}
	}
	want := []string{
		"session", "usage out=112", "tool_call Bash", "tool_result", "text started.", "usage out=4", "turn_end",
		"session", "text Tides are the", "text  r", "usage out=546", "turn_end",
		"session", "usage out=45",
		"session", "text 42", "usage out=3", "turn_end",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestParse_turnUsage(t *testing.T) {
	delta := func(in, cw, cr, out int) string {
		return fmt.Sprintf(`{"type":"stream_event","event":{"type":"message_delta","usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":%d}},"parent_tool_use_id":null}`, in, cw, cr, out)
	}
	result := `{"type":"result","subtype":"success","is_error":false,"result":"done","usage":{"input_tokens":8,"cache_creation_input_tokens":204,"cache_read_input_tokens":70674,"output_tokens":214}}`
	tests := []struct {
		name string
		give []string
		want []agent.Usage
	}{
		{
			name: "messages add up to the result",
			give: []string{delta(2, 0, 15957, 65), delta(2, 0, 18205, 72), delta(2, 102, 18205, 72), delta(2, 102, 18307, 5), result},
			want: []agent.Usage{
				{InputTokens: 2, OutputTokens: 65, CacheReadTokens: 15957},
				{InputTokens: 2, OutputTokens: 72, CacheReadTokens: 18205},
				{InputTokens: 2, OutputTokens: 72, CacheReadTokens: 18205, CacheWriteTokens: 102},
				{InputTokens: 2, OutputTokens: 5, CacheReadTokens: 18307, CacheWriteTokens: 102},
			},
		},
		{
			name: "result adds what the messages missed",
			give: []string{delta(2, 0, 15957, 65), result},
			want: []agent.Usage{
				{InputTokens: 2, OutputTokens: 65, CacheReadTokens: 15957},
				{InputTokens: 6, OutputTokens: 149, CacheReadTokens: 54717, CacheWriteTokens: 204},
			},
		},
		{
			name: "a new turn starts from zero",
			give: []string{delta(2, 0, 15957, 65), result, delta(1, 0, 3, 4), result},
			want: []agent.Usage{
				{InputTokens: 2, OutputTokens: 65, CacheReadTokens: 15957},
				{InputTokens: 6, OutputTokens: 149, CacheReadTokens: 54717, CacheWriteTokens: 204},
				{InputTokens: 1, OutputTokens: 4, CacheReadTokens: 3},
				{InputTokens: 7, OutputTokens: 210, CacheReadTokens: 70671, CacheWriteTokens: 204},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var st state
			var got []agent.Usage
			for _, line := range tt.give {
				for _, ev := range parse([]byte(line), &st) {
					if ev.Kind == agent.KindUsage {
						got = append(got, *ev.Usage)
					}
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("usage events = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func fakeClaude(t *testing.T, scenario string) *Agent {
	t.Helper()
	dir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("FAKE_CLAUDE", scenario)
	a := New()
	t.Cleanup(func() { _ = a.Interrupt() })
	return a
}

// drain collects events in the background; the result arrives once Events is closed.
func drain(a *Agent) <-chan []agent.Event {
	out := make(chan []agent.Event, 1)
	go func() {
		var got []agent.Event
		for ev := range a.Events() {
			got = append(got, ev)
		}
		out <- got
	}()
	return out
}

func collect(t *testing.T, out <-chan []agent.Event) []agent.Event {
	t.Helper()
	select {
	case got := <-out:
		return got
	case <-time.After(10 * time.Second):
		t.Fatal("Events not closed")
		return nil
	}
}

func TestInterruptMidTurn(t *testing.T) {
	for range 20 {
		a := fakeClaude(t, "hang")
		cfg := agent.Config{Cwd: t.TempDir()}
		if err := a.Start(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
		if err := a.Start(context.Background(), cfg); err == nil {
			t.Fatal("second Start succeeded")
		}
		first := <-a.Events()
		if first.Kind != agent.KindSession || first.SessionID != "s1" {
			t.Fatalf("first event = %+v", first)
		}
		out := drain(a)
		begin := time.Now()
		if err := a.Interrupt(); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(begin); d > 2*time.Second {
			t.Fatalf("Interrupt took %v", d)
		}
		if got := collect(t, out); len(got) != 0 {
			t.Fatalf("events after Interrupt = %+v", got)
		}
	}
}

func TestOversizedLine(t *testing.T) {
	a := fakeClaude(t, "big")
	if err := a.Start(context.Background(), agent.Config{Cwd: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	want := []agent.Event{
		{Kind: agent.KindToolResult, ToolID: "t1", Output: "done"},
		{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 1, OutputTokens: 2, CacheReadTokens: 4, CacheWriteTokens: 3}},
		{Kind: agent.KindTurnEnd},
	}
	if got := collect(t, drain(a)); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

func TestFailedResume(t *testing.T) {
	a := fakeClaude(t, "badresume")
	if err := a.Resume(context.Background(), agent.Config{Cwd: t.TempDir()}, "x"); err != nil {
		t.Fatal(err)
	}
	want := []agent.Event{
		{Kind: agent.KindError, Text: "No conversation found"},
	}
	if got := collect(t, drain(a)); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

func TestResumeAfterBackgroundTask(t *testing.T) {
	a := fakeClaude(t, "resumebg")
	if err := a.Resume(context.Background(), agent.Config{Cwd: t.TempDir()}, "d6c291c5"); err != nil {
		t.Fatal(err)
	}
	if err := a.Send(context.Background(), "What is 7 times 6?"); err != nil {
		t.Fatal(err)
	}
	want := []agent.Event{
		{Kind: agent.KindSession, SessionID: "d6c291c5-ec96-403e-9f97-720d1dee78db"},
		{Kind: agent.KindSession, SessionID: "d6c291c5-ec96-403e-9f97-720d1dee78db"},
		{Kind: agent.KindText, Text: "42"},
		{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 2, OutputTokens: 3, CacheReadTokens: 18362, CacheWriteTokens: 425}},
		{Kind: agent.KindTurnEnd},
	}
	if got := collect(t, drain(a)); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

func TestMCPServersPreApproved(t *testing.T) {
	a := fakeClaude(t, "argv")
	cfg := agent.Config{
		Cwd:        t.TempDir(),
		MCPServers: map[string]agent.MCPServer{"reef": {Command: "r"}, "ping": {URL: "http://x"}},
	}
	if err := a.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got := collect(t, drain(a))
	if len(got) != 1 || !strings.Contains(got[0].Output, " --allowedTools mcp__ping__*,mcp__reef__*") {
		t.Errorf("events = %+v", got)
	}
}

func TestMCPServerNameRejected(t *testing.T) {
	a := fakeClaude(t, "argv")
	cfg := agent.Config{
		Cwd:        t.TempDir(),
		MCPServers: map[string]agent.MCPServer{"x,Bash,y": {Command: "r"}},
	}
	err := a.Start(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), `"x,Bash,y"`) {
		t.Fatalf("Start error = %v", err)
	}
	if err := a.Interrupt(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-a.Events(); ok {
		t.Error("Events is open")
	}
}

func TestInterruptBeforeStart(t *testing.T) {
	a := New()
	if err := a.Interrupt(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-a.Events(); ok {
		t.Error("Events is open")
	}
	if err := a.Interrupt(); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(context.Background(), agent.Config{Cwd: t.TempDir()}); err == nil {
		t.Error("Start after Interrupt succeeded")
	}
}
