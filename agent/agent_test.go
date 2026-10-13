package agent

import (
	"encoding/json"
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
