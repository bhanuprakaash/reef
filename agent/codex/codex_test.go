package codex

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

func TestMessage_event(t *testing.T) {
	tests := []struct {
		name string
		give string
		want *agent.Event // nil: skipped
	}{
		{
			name: "text delta",
			give: `{"method":"item/agentMessage/delta","params":{"threadId":"01a133d7","turnId":"01a133d7","itemId":"msg_057e","delta":"I"}}`,
			want: &agent.Event{Kind: agent.KindText, Text: "I"},
		},
		{
			name: "command start",
			give: `{"method":"item/started","params":{"item":{"type":"commandExecution","id":"exec-0ff8","command":"/bin/zsh -lc 'ls -la'","cwd":"/tmp/cx","status":"inProgress","aggregatedOutput":null,"exitCode":null}}}`,
			want: &agent.Event{
				Kind:     agent.KindToolCall,
				ToolID:   "exec-0ff8",
				ToolName: "commandExecution",
				Input:    json.RawMessage(`{"command":"/bin/zsh -lc 'ls -la'"}`),
			},
		},
		{
			name: "command done",
			give: `{"method":"item/completed","params":{"item":{"type":"commandExecution","id":"exec-0ff8","command":"/bin/zsh -lc 'ls -la'","cwd":"/tmp/cx","status":"completed","aggregatedOutput":"a.txt\nb.go\n","exitCode":0}}}`,
			want: &agent.Event{Kind: agent.KindToolResult, ToolID: "exec-0ff8", Output: "a.txt\nb.go\n"},
		},
		{
			name: "file change start",
			give: `{"method":"item/started","params":{"item":{"type":"fileChange","id":"exec-69db","changes":[{"path":"/tmp/cx/notes2.md","kind":{"type":"add"},"diff":"x\n"}],"status":"inProgress"}}}`,
			want: &agent.Event{
				Kind:     agent.KindToolCall,
				ToolID:   "exec-69db",
				ToolName: "fileChange",
				Input:    json.RawMessage(`[{"path":"/tmp/cx/notes2.md","kind":{"type":"add"},"diff":"x\n"}]`),
			},
		},
		{
			name: "file change done",
			give: `{"method":"item/completed","params":{"item":{"type":"fileChange","id":"exec-69db","changes":[{"path":"/tmp/cx/notes2.md","kind":{"type":"add"},"diff":"x\n"}],"status":"completed"}}}`,
			want: &agent.Event{Kind: agent.KindToolResult, ToolID: "exec-69db", Output: "completed"},
		},
		{
			name: "mcp start",
			give: `{"method":"item/started","params":{"item":{"type":"mcpToolCall","id":"exec-fb96","server":"reef-ping","tool":"reef_ping","status":"inProgress","arguments":{},"result":null,"error":null}}}`,
			want: &agent.Event{
				Kind:     agent.KindToolCall,
				ToolID:   "exec-fb96",
				ToolName: "mcp__reef-ping__reef_ping",
				Input:    json.RawMessage(`{}`),
			},
		},
		{
			name: "mcp done",
			give: `{"method":"item/completed","params":{"item":{"type":"mcpToolCall","id":"exec-fb96","server":"reef-ping","tool":"reef_ping","status":"completed","arguments":{},"result":{"content":[{"type":"text","text":"pong"}],"structuredContent":null},"error":null}}}`,
			want: &agent.Event{Kind: agent.KindToolResult, ToolID: "exec-fb96", Output: "pong"},
		},
		{
			name: "mcp refused",
			give: `{"method":"item/completed","params":{"item":{"type":"mcpToolCall","id":"exec-2d12","server":"reef-ping","tool":"reef_ping","status":"failed","arguments":{},"result":null,"error":{"message":"MCP tool call requires approval, but approval policy is never"}}}}`,
			want: &agent.Event{
				Kind:    agent.KindToolResult,
				ToolID:  "exec-2d12",
				Output:  "MCP tool call requires approval, but approval policy is never",
				IsError: true,
			},
		},
		{
			name: "usage",
			give: `{"method":"thread/tokenUsage/updated","params":{"threadId":"01a133d7","turnId":"01a133d7","tokenUsage":{"total":{"totalTokens":30233,"inputTokens":30129,"cachedInputTokens":15872,"outputTokens":104},"last":{"totalTokens":15144,"inputTokens":15026,"cachedInputTokens":14080,"cacheWriteInputTokens":0,"outputTokens":118,"reasoningOutputTokens":45},"modelContextWindow":258400}}}`,
			want: &agent.Event{
				Kind:  agent.KindUsage,
				Usage: &agent.Usage{InputTokens: 946, OutputTokens: 118, CacheReadTokens: 14080},
			},
		},
		{
			name: "usage with cache write",
			give: `{"method":"thread/tokenUsage/updated","params":{"tokenUsage":{"last":{"totalTokens":15144,"inputTokens":15026,"cachedInputTokens":14080,"cacheWriteInputTokens":500,"outputTokens":118,"reasoningOutputTokens":45}}}}`,
			want: &agent.Event{
				Kind: agent.KindUsage,
				Usage: &agent.Usage{
					InputTokens: 446, OutputTokens: 118, CacheReadTokens: 14080, CacheWriteTokens: 500,
				},
			},
		},
		{
			name: "turn end",
			give: `{"method":"turn/completed","params":{"threadId":"01a133d7","turn":{"id":"01a133d7","itemsView":"summary","status":"completed","error":null,"durationMs":8537}}}`,
			want: &agent.Event{Kind: agent.KindTurnEnd},
		},
		{
			name: "turn failed",
			give: `{"method":"turn/completed","params":{"threadId":"01a133d8","turn":{"id":"01a133d8","itemsView":"notLoaded","status":"failed","error":{"message":"model not supported","codexErrorInfo":"other"},"durationMs":3144}}}`,
			want: &agent.Event{Kind: agent.KindError, Text: "model not supported"},
		},
		{
			name: "request refused",
			give: `{"id":3,"error":{"code":-32600,"message":"Invalid request: unknown variant"}}`,
			want: &agent.Event{Kind: agent.KindError, Text: "Invalid request: unknown variant"},
		},
		{
			name: "error notification",
			give: `{"method":"error","params":{"error":{"message":"model not supported"},"willRetry":false,"threadId":"01a133d8","turnId":"01a133d8"}}`,
		},
		{
			name: "user message",
			give: `{"method":"item/started","params":{"item":{"type":"userMessage","id":"01a133d7","content":[{"type":"text","text":"list files"}]}}}`,
		},
		{
			name: "reasoning",
			give: `{"method":"item/completed","params":{"item":{"type":"reasoning","id":"rs_02b4","summary":[],"content":[]}}}`,
		},
		{
			name: "mcp status",
			give: `{"method":"mcpServer/startupStatus/updated","params":{"threadId":"01a133d1","name":"codex_apps","status":"ready","error":null}}`,
		},
		{
			name: "response",
			give: `{"id":3,"result":{"turn":{"id":"01a133d7","items":[],"status":"inProgress","error":null}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m message
			if err := json.Unmarshal([]byte(tt.give), &m); err != nil {
				t.Fatal(err)
			}
			got, ok := m.event()
			if tt.want == nil {
				if ok {
					t.Errorf("event() = %+v, want skipped", got)
				}
				return
			}
			if !ok || !reflect.DeepEqual(got, *tt.want) {
				t.Errorf("event() = %+v, %v; want %+v", got, ok, *tt.want)
			}
		})
	}
}

func fake(t *testing.T, mode string) *Agent {
	t.Helper()
	dir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CODEX", mode)
	a := New()
	t.Cleanup(func() { _ = a.Interrupt() })
	return a
}

func recv(t *testing.T, a *Agent) (agent.Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-a.Events():
		return ev, ok
	case <-time.After(2 * time.Second):
		t.Fatal("no event or close within 2s")
		return agent.Event{}, false
	}
}

func collect(t *testing.T, a *Agent, stop agent.Kind) []agent.Event {
	t.Helper()
	var got []agent.Event
	for {
		ev, ok := recv(t, a)
		if !ok {
			return got
		}
		got = append(got, ev)
		if ev.Kind == stop {
			return got
		}
	}
}

func startTurn(t *testing.T, a *Agent) {
	t.Helper()
	if err := a.Start(context.Background(), agent.Config{Cwd: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := a.Send(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
}

func TestThreadParams_Sandbox(t *testing.T) {
	params := threadParams(agent.Config{Cwd: "/work", PermissionMode: "read-only"})
	if params["sandbox"] != "read-only" || params["cwd"] != "/work" {
		t.Errorf("threadParams = %v, want sandbox read-only in /work", params)
	}
}

func TestThreadParams_MCPApprovalMode(t *testing.T) {
	params := threadParams(agent.Config{MCPServers: map[string]agent.MCPServer{
		"reef": {Command: "reef-mcp"},
		"mcp1": {Command: "other-mcp"},
	}})
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Config struct {
			MCPServers map[string]map[string]any `json:"mcp_servers"`
		} `json:"config"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Config.MCPServers["reef"]["default_tools_approval_mode"] != "approve" {
		t.Errorf("reef approval mode = %v, want approve", got.Config.MCPServers["reef"])
	}
	if _, ok := got.Config.MCPServers["mcp1"]["default_tools_approval_mode"]; ok {
		t.Errorf("mcp1 has approval mode: %v", got.Config.MCPServers["mcp1"])
	}
}

func TestAgent_Interrupt(t *testing.T) {
	a := fake(t, "hang")
	startTurn(t, a)
	collect(t, a, agent.KindText)

	begin := time.Now()
	if err := a.Interrupt(); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(begin); d > 2*time.Second {
		t.Errorf("Interrupt took %v, want under 2s", d)
	}
	if ev, ok := recv(t, a); ok {
		t.Errorf("event after Interrupt: %+v, want Events closed", ev)
	}
	if err := a.Interrupt(); err != nil {
		t.Errorf("second Interrupt = %v, want nil", err)
	}
}

func TestAgent_StartFails(t *testing.T) {
	a := fake(t, "failstart")
	err := a.Start(context.Background(), agent.Config{Cwd: t.TempDir()})
	if err == nil || err.Error() != "thread/start: bad sandbox" {
		t.Fatalf("Start = %v, want thread/start: bad sandbox", err)
	}
	if ev, ok := recv(t, a); ok {
		t.Errorf("event after failed Start: %+v, want Events closed", ev)
	}
	if err := a.Interrupt(); err != nil {
		t.Errorf("Interrupt = %v, want nil", err)
	}
}

func TestAgent_InterruptDuringStart(t *testing.T) {
	fake(t, "hang")
	for i := range 200 {
		a := New()
		done := make(chan struct{})
		go func() {
			defer close(done)
			time.Sleep(time.Duration(i%5) * time.Millisecond)
			_ = a.Interrupt()
		}()
		_ = a.Start(context.Background(), agent.Config{Cwd: t.TempDir()})
		<-done
		if ev, ok := recv(t, a); ok {
			t.Fatalf("event after Interrupt: %+v, want Events closed", ev)
		}
	}
}

func TestAgent_StartCancelled(t *testing.T) {
	a := fake(t, "slowstart")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	err := a.Start(ctx, agent.Config{Cwd: t.TempDir()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start = %v, want context.Canceled", err)
	}
	if ev, ok := recv(t, a); ok {
		t.Errorf("event after cancelled Start: %+v, want Events closed", ev)
	}
}

func TestAgent_StartCancelledIgnoringSIGTERM(t *testing.T) {
	a := fake(t, "deafstart")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	begin := time.Now()
	err := a.Start(ctx, agent.Config{Cwd: t.TempDir()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start = %v, want context.Canceled", err)
	}
	if d := time.Since(begin); d > 2*time.Second {
		t.Errorf("Start took %v after cancel, want under 2s", d)
	}
}

func TestAgent_StartCancelledNearReady(t *testing.T) {
	fake(t, "hang")
	for range 200 {
		a := New()
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(time.Duration(rand.IntN(8000))*time.Microsecond, cancel)
		err := a.Start(ctx, agent.Config{Cwd: t.TempDir()})
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Start = %v, want nil or context.Canceled", err)
			}
			continue
		}
		deadline := time.After(20 * time.Millisecond)
	drain:
		for {
			select {
			case ev, ok := <-a.Events():
				if !ok {
					break drain
				}
				if ev.Kind == agent.KindError {
					t.Fatalf("Start = nil, then error event %q", ev.Text)
				}
			case <-deadline:
				break drain
			}
		}
		_ = a.Interrupt()
	}
}

func TestAgent_ExitsOnItsOwn(t *testing.T) {
	a := fake(t, "crash")
	startTurn(t, a)

	got := collect(t, a, "")
	want := []agent.Event{
		{Kind: agent.KindSession, SessionID: "t1"},
		{Kind: agent.KindText, Text: "hi"},
		{Kind: agent.KindTurnEnd},
		{Kind: agent.KindError, Text: "codex exited: exit status 3: boom"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
	if a.cmd.ProcessState == nil {
		t.Error("process not reaped after it exited")
	}
	if err := a.Interrupt(); err != nil {
		t.Errorf("Interrupt = %v, want nil", err)
	}
}

func TestAgent_ResumeAfterInterrupt(t *testing.T) {
	a := fake(t, "hang")
	startTurn(t, a)
	if err := a.Interrupt(); err != nil {
		t.Fatal(err)
	}
	if err := a.Resume(context.Background(), agent.Config{Cwd: t.TempDir()}, "t1"); err == nil {
		t.Error("Resume on a used Agent = nil, want error")
	}
}

func TestAgent_NotStarted(t *testing.T) {
	a := New()
	if err := a.Send(context.Background(), "go"); err == nil {
		t.Error("Send before Start = nil, want error")
	}
	if err := a.Interrupt(); err != nil {
		t.Errorf("Interrupt = %v, want nil", err)
	}
	if ev, ok := recv(t, a); ok {
		t.Errorf("event: %+v, want Events closed", ev)
	}
}

func TestAgent_UsageAsItArrives(t *testing.T) {
	a := fake(t, "twocalls")
	startTurn(t, a)
	got := collect(t, a, agent.KindTurnEnd)
	want := []agent.Event{
		{Kind: agent.KindSession, SessionID: "t1"},
		{Kind: agent.KindText, Text: "hi"},
		{Kind: agent.KindUsage, Usage: &agent.Usage{
			InputTokens: 4, OutputTokens: 3, CacheReadTokens: 4, CacheWriteTokens: 2,
		}},
		{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 6, OutputTokens: 5, CacheReadTokens: 1}},
		{Kind: agent.KindTurnEnd},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

func TestAgent_ResumeDropsReplayedUsage(t *testing.T) {
	a := fake(t, "replay")
	if err := a.Resume(context.Background(), agent.Config{Cwd: t.TempDir()}, "t1"); err != nil {
		t.Fatal(err)
	}
	if err := a.Send(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	got := collect(t, a, agent.KindTurnEnd)
	want := []agent.Event{
		{Kind: agent.KindSession, SessionID: "t1"},
		{Kind: agent.KindText, Text: "hi"},
		{Kind: agent.KindUsage, Usage: &agent.Usage{
			InputTokens: 4, OutputTokens: 3, CacheReadTokens: 4, CacheWriteTokens: 2,
		}},
		{Kind: agent.KindTurnEnd},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

func TestAgent_TurnError(t *testing.T) {
	tests := []struct {
		mode string
		want []agent.Event
	}{
		{
			mode: "rejected",
			want: []agent.Event{
				{Kind: agent.KindSession, SessionID: "t1"},
				{Kind: agent.KindError, Text: "bad turn"},
			},
		},
		{
			mode: "midcrash",
			want: []agent.Event{
				{Kind: agent.KindSession, SessionID: "t1"},
				{Kind: agent.KindText, Text: "hi"},
				{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 6, OutputTokens: 3, CacheReadTokens: 4}},
				{Kind: agent.KindError, Text: "codex exited: exit status 3: boom"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			a := fake(t, tt.mode)
			startTurn(t, a)
			if got := collect(t, a, agent.KindError); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("events = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAgent_InterruptedTurnKeepsUsage(t *testing.T) {
	a := fake(t, "usagehang")
	startTurn(t, a)
	got := collect(t, a, agent.KindUsage)
	want := []agent.Event{
		{Kind: agent.KindSession, SessionID: "t1"},
		{Kind: agent.KindText, Text: "hi"},
		{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 6, OutputTokens: 3, CacheReadTokens: 4}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
	if err := a.Interrupt(); err != nil {
		t.Fatal(err)
	}
}

func TestAgent_InterruptKeepsStoppedTurnUsage(t *testing.T) {
	tests := []struct {
		mode string
		want agent.Usage
	}{
		{mode: "stopusage", want: agent.Usage{InputTokens: 6, OutputTokens: 5, CacheReadTokens: 1}},
		{mode: "stopmerge", want: agent.Usage{InputTokens: 12, OutputTokens: 8, CacheReadTokens: 5}},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			a := fake(t, tt.mode)
			startTurn(t, a)
			collect(t, a, agent.KindToolCall)
			if err := a.Interrupt(); err != nil {
				t.Fatal(err)
			}
			got := collect(t, a, "")
			want := []agent.Event{{Kind: agent.KindUsage, Usage: &tt.want}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("events after Interrupt = %+v, want %+v", got, want)
			}
		})
	}
}

func TestAgent_BreakBetweenMessages(t *testing.T) {
	a := fake(t, "twomsgs")
	startTurn(t, a)
	got := collect(t, a, agent.KindTurnEnd)
	want := []agent.Event{
		{Kind: agent.KindSession, SessionID: "t1"},
		{Kind: agent.KindText, Text: "hi"},
		{Kind: agent.KindText, Text: "a1"},
		{Kind: agent.KindText, Text: "a2"},
		{
			Kind: agent.KindToolCall, ToolID: "c1", ToolName: "commandExecution",
			Input: json.RawMessage(`{"command":"ls"}`),
		},
		{Kind: agent.KindToolResult, ToolID: "c1", Output: "x"},
		{Kind: agent.KindText, Text: "\n\n"},
		{Kind: agent.KindText, Text: "b1"},
		{Kind: agent.KindTurnEnd},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}
