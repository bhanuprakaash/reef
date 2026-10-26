package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEvent_JSON(t *testing.T) {
	tests := []struct {
		name string
		give Event
		want string
	}{
		{
			name: "text",
			give: Event{Kind: KindText, Text: "hello"},
			want: `{"kind":"text","text":"hello"}`,
		},
		{
			name: "tool call",
			give: Event{
				Kind:     KindToolCall,
				ToolID:   "t1",
				ToolName: "Read",
				Input:    json.RawMessage(`{"path":"a.go"}`),
			},
			want: `{"kind":"tool_call","toolId":"t1","toolName":"Read","input":{"path":"a.go"}}`,
		},
		{
			name: "tool result",
			give: Event{Kind: KindToolResult, ToolID: "t1", Output: "boom", IsError: true},
			want: `{"kind":"tool_result","toolId":"t1","output":"boom","isError":true}`,
		},
		{
			name: "subagent tool call",
			give: Event{Kind: KindToolCall, ToolID: "t2", ToolName: "Bash", ParentToolID: "t1"},
			want: `{"kind":"tool_call","toolId":"t2","toolName":"Bash","parentToolId":"t1"}`,
		},
		{
			name: "usage",
			give: Event{
				Kind:  KindUsage,
				Usage: &Usage{InputTokens: 10, OutputTokens: 20, CacheReadTokens: 5, CacheWriteTokens: 7},
			},
			want: `{"kind":"usage","usage":{"inputTokens":10,"outputTokens":20,"cacheReadTokens":5,"cacheWriteTokens":7}}`,
		},
		{
			name: "turn end",
			give: Event{Kind: KindTurnEnd},
			want: `{"kind":"turn_end"}`,
		},
		{
			name: "error",
			give: Event{Kind: KindError, Text: "backend exited"},
			want: `{"kind":"error","text":"backend exited"}`,
		},
		{
			name: "session",
			give: Event{Kind: KindSession, SessionID: "s1"},
			want: `{"kind":"session","sessionId":"s1"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.give)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTail_keepsLast4KB(t *testing.T) {
	var tl Tail
	chunk := strings.Repeat("x", 1<<20)
	for range 10 {
		if _, err := tl.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	tl.Write([]byte("boom"))
	if got, want := len(tl.Bytes()), 4096; got != want {
		t.Errorf("len = %d, want %d", got, want)
	}
	if got := string(tl.Bytes()[len(tl.Bytes())-4:]); got != "boom" {
		t.Errorf("tail = %q, want %q", got, "boom")
	}
}

func TestUsage_Sub(t *testing.T) {
	u := Usage{InputTokens: 5, OutputTokens: 1, CacheReadTokens: 10, CacheWriteTokens: 0}
	u.Sub(Usage{InputTokens: 2, OutputTokens: 3, CacheReadTokens: 10, CacheWriteTokens: 4})
	if want := (Usage{InputTokens: 3, CacheReadTokens: 0}); u != want {
		t.Errorf("Sub = %+v, want %+v", u, want)
	}
}
