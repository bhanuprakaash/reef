package claude

import (
	"context"
	"encoding/json"
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
			name: "tool call",
			give: `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01QXVfbj1JZFm29Aqmf9Gb8z","name":"Bash","input":{"command":"echo hello-from-tool","description":"Print test string"},"caller":{"type":"direct"}}]},"parent_tool_use_id":null,"session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381"}`,
			want: []agent.Event{{
				Kind:     agent.KindToolCall,
				ToolID:   "toolu_01QXVfbj1JZFm29Aqmf9Gb8z",
				ToolName: "Bash",
				Input:    json.RawMessage(`{"command":"echo hello-from-tool","description":"Print test string"}`),
			}},
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
				{Kind: agent.KindUsage, Usage: &agent.Usage{}},
				{Kind: agent.KindError, Text: "No conversation found with session ID: 00000000-0000-0000-0000-000000000000"},
			},
		},
		{
			name: "not json",
			give: `claude: warming up`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parse([]byte(tt.give))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parse() = %+v, want %+v", got, tt.want)
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
		{Kind: agent.KindUsage, Usage: &agent.Usage{}},
		{Kind: agent.KindError, Text: "No conversation found"},
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
