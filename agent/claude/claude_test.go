package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

var resultCost = 0.0123
var errorCost = 0.0073

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		give string
		want []agent.Event
	}{
		{
			name: "init",
			give: `{"type":"system","subtype":"init","cwd":"/private/tmp/scratch-claude","session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381","model":"claude-opus-5-5","permissionMode":"bypassPermissions"}`,
			want: []agent.Event{{Kind: agent.KindSession, SessionID: "3c1296cc-1e28-4167-a8a8-dfb0facf9381", Model: "claude-opus-5-5"}},
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
			name: "image tool result",
			give: `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01Img","type":"tool_result","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}]}]},"parent_tool_use_id":null,"session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381"}`,
			want: []agent.Event{{Kind: agent.KindToolResult, ToolID: "toolu_01Img", Output: "[image]"}},
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
			give: `{"type":"result","subtype":"success","is_error":false,"result":"Output: hello-from-tool","session_id":"3c1296cc-1e28-4167-a8a8-dfb0facf9381","total_cost_usd":0.0123,"usage":{"input_tokens":4,"cache_creation_input_tokens":10112,"cache_read_input_tokens":22898,"output_tokens":123},"modelUsage":{"claude-opus-5-5":{"inputTokens":4,"outputTokens":123,"cacheReadInputTokens":22898,"cacheCreationInputTokens":10112},"claude-haiku-5-5":{"inputTokens":1200,"outputTokens":20,"cacheReadInputTokens":0,"cacheCreationInputTokens":0}}}`,
			want: []agent.Event{
				{
					Kind: agent.KindTurnEnd, CostUSD: &resultCost,
					SessionUsage: &agent.Usage{InputTokens: 1204, OutputTokens: 143, CacheReadTokens: 22898, CacheWriteTokens: 10112},
				},
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
			name: "error result with cost",
			give: `{"type":"result","subtype":"error_during_execution","is_error":true,"session_id":"00000000-0000-0000-0000-000000000000","total_cost_usd":0.0073,"usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0},"errors":["boom"]}`,
			want: []agent.Event{
				{Kind: agent.KindError, Text: "boom", CostUSD: &errorCost},
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

func TestParse_resultHeldWithItsCall(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/tmp/note.txt"}}]},"parent_tool_use_id":null}`,
		`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01","type":"tool_result","content":"note","is_error":false}]},"parent_tool_use_id":null}`,
		`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":2,"cache_creation_input_tokens":8139,"cache_read_input_tokens":7817,"output_tokens":63}},"parent_tool_use_id":null}`,
	}
	want := []agent.Event{
		{Kind: agent.KindUsage, Usage: &agent.Usage{InputTokens: 2, OutputTokens: 63, CacheReadTokens: 7817, CacheWriteTokens: 8139}},
		{Kind: agent.KindToolCall, ToolID: "toolu_01", ToolName: "Read", Input: json.RawMessage(`{"file_path":"/tmp/note.txt"}`)},
		{Kind: agent.KindToolResult, ToolID: "toolu_01", Output: "note"},
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

// parallel-tools.jsonl is a real turn, trimmed: three Reads in one message. The result of
// the first arrives before the message_delta that ends the message.
func TestParse_resultsBeforeMessageUsage(t *testing.T) {
	raw, err := os.ReadFile("testdata/parallel-tools.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var st state
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		for _, ev := range parse([]byte(line), &st) {
			s := string(ev.Kind)
			switch ev.Kind {
			case agent.KindToolCall, agent.KindToolResult:
				s += " " + ev.ToolID[len(ev.ToolID)-4:]
			case agent.KindUsage:
				s += fmt.Sprintf(" out=%d", ev.Usage.OutputTokens)
			}
			got = append(got, s)
		}
	}
	want := []string{
		"session", "usage out=186", "tool_call MH73", "tool_result MH73",
		"tool_call d7DQ", "tool_call gnMD", "tool_result gnMD", "tool_result d7DQ",
		"usage out=5", "turn_end",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %q, want %q", got, want)
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
			case agent.KindText, agent.KindBackground:
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
		"session", "usage out=45", "background The background command finished with",
		"session", "text 42", "usage out=3", "turn_end",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func replayFile(t *testing.T, name string, st *state) []agent.Event {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return replay(strings.Split(strings.TrimSpace(string(raw)), "\n"), st)
}

func replay(lines []string, st *state) []agent.Event {
	var got []agent.Event
	for _, line := range lines {
		got = append(got, parse([]byte(line), st)...)
	}
	return got
}

// sessionUsage is the SessionUsage of the last turn end of evs, and streamed the sum of the
// usage events before it.
func sessionUsage(t *testing.T, evs []agent.Event) (session *agent.Usage, streamed agent.Usage) {
	t.Helper()
	for _, ev := range evs {
		switch ev.Kind {
		case agent.KindUsage:
			u := *ev.Usage
			if u.InputTokens < 0 || u.OutputTokens < 0 || u.CacheReadTokens < 0 || u.CacheWriteTokens < 0 {
				t.Errorf("negative usage event %+v", u)
			}
			streamed.Add(u)
		case agent.KindTurnEnd:
			session = ev.SessionUsage
		}
	}
	return session, streamed
}

// The turn end of a real session carries the modelUsage of its last result. The messages'
// usage events leave out the subagents (subagent.jsonl) and claude's own side calls, such as
// the session title, which modelUsage counts under a model of their own (the haiku lines of
// the two others).
func TestParse_turnEndHoldsModelUsage(t *testing.T) {
	tests := []struct {
		file string
		want agent.Usage
	}{
		{"subagent.jsonl", agent.Usage{InputTokens: 8, OutputTokens: 511, CacheReadTokens: 53174, CacheWriteTokens: 24389}},
		{"background-task.jsonl", agent.Usage{InputTokens: 1216, OutputTokens: 728, CacheReadTokens: 79876, CacheWriteTokens: 11594}},
		{"parallel-tools.jsonl", agent.Usage{InputTokens: 1204, OutputTokens: 207, CacheReadTokens: 32323, CacheWriteTokens: 7363}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got, streamed := sessionUsage(t, replayFile(t, "testdata/"+tt.file, &state{}))
			if got == nil || *got != tt.want {
				t.Fatalf("session usage = %+v, want %+v", got, tt.want)
			}
			if streamed == *got {
				t.Errorf("streamed usage = session usage %+v: the fixture holds no subagent or side call", streamed)
			}
		})
	}
}

// A resumed claude's modelUsage is the whole session's, earlier processes included, so the
// turn end sends it as it is: the subagent of this turn is in it. subagent.jsonl is a real
// first turn with a subagent; base is what an earlier turn of the session had spent, which a
// resume adds to every model's count. Taking base off the turn end gives back the turn's own
// usage, subagent included.
func TestParse_resumedModelUsageHoldsSubagent(t *testing.T) {
	raw, err := os.ReadFile("testdata/subagent.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var result map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &result); err != nil {
		t.Fatal(err)
	}
	base := agent.Usage{InputTokens: 1176, OutputTokens: 23, CacheReadTokens: 38208, CacheWriteTokens: 7551}
	counts, ok := result["modelUsage"].(map[string]any)["claude-sonnet-5-5"].(map[string]any)
	if !ok {
		t.Fatal("the fixture's result has no modelUsage for claude-sonnet-5-5")
	}
	counts["inputTokens"] = counts["inputTokens"].(float64) + float64(base.InputTokens)
	counts["outputTokens"] = counts["outputTokens"].(float64) + float64(base.OutputTokens)
	counts["cacheReadInputTokens"] = counts["cacheReadInputTokens"].(float64) + float64(base.CacheReadTokens)
	counts["cacheCreationInputTokens"] = counts["cacheCreationInputTokens"].(float64) + float64(base.CacheWriteTokens)
	resumed, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	lines[len(lines)-1] = string(resumed)

	got, _ := sessionUsage(t, replay(lines, &state{}))
	if got == nil {
		t.Fatal("turn end has no session usage")
	}
	own := *got
	own.Sub(base)
	want := agent.Usage{InputTokens: 8, OutputTokens: 511, CacheReadTokens: 53174, CacheWriteTokens: 24389}
	if own != want {
		t.Errorf("session usage minus the earlier turns = %+v, want %+v (the whole turn, subagent included)", own, want)
	}
}

// The notification turn of background-task.jsonl starts before the result of the turn that
// was running when the task ended. It sends its usage first, and no usage event is negative.
// The session usage of the later turn end still counts it.
func TestParse_earlyBackgroundTurn(t *testing.T) {
	raw, err := os.ReadFile("testdata/background-task.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	l := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var lines []string
	lines = append(lines, l[:35]...)   // turns 1 and 2 up to the result of turn 2
	lines = append(lines, l[36:45]...) // the notification turn up to its result
	lines = append(lines, l[35], l[45])
	lines = append(lines, l[46:]...)
	got, _ := sessionUsage(t, replay(lines, &state{}))
	want := agent.Usage{InputTokens: 1216, OutputTokens: 728, CacheReadTokens: 79876, CacheWriteTokens: 11594}
	if got == nil || *got != want {
		t.Errorf("session usage = %+v, want %+v", got, want)
	}
}

func TestParse_messageUsage(t *testing.T) {
	delta := func(in, cw, cr, out int) string {
		return fmt.Sprintf(`{"type":"stream_event","event":{"type":"message_delta","usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":%d}},"parent_tool_use_id":null}`, in, cw, cr, out)
	}
	result := `{"type":"result","subtype":"success","is_error":false,"result":"done","modelUsage":{"claude-sonnet-5-5":{"inputTokens":6,"outputTokens":149,"cacheReadInputTokens":54717,"cacheCreationInputTokens":204}}}`
	var st state
	var got []agent.Usage
	for _, line := range []string{delta(2, 0, 15957, 65), delta(2, 102, 18205, 72), result} {
		for _, ev := range parse([]byte(line), &st) {
			switch ev.Kind {
			case agent.KindUsage:
				got = append(got, *ev.Usage)
			case agent.KindTurnEnd:
				got = append(got, *ev.SessionUsage)
			}
		}
	}
	want := []agent.Usage{
		{InputTokens: 2, OutputTokens: 65, CacheReadTokens: 15957},
		{InputTokens: 2, OutputTokens: 72, CacheReadTokens: 18205, CacheWriteTokens: 102},
		{InputTokens: 6, OutputTokens: 149, CacheReadTokens: 54717, CacheWriteTokens: 204},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("usage = %+v, want %+v: each message as it ends, then the session total", got, want)
	}
}

func TestParse_costIsSessionTotal(t *testing.T) {
	turn := func(cost float64) string {
		return fmt.Sprintf(`{"type":"result","subtype":"success","is_error":false,"result":"done","total_cost_usd":%g}`, cost)
	}
	background := `{"type":"result","subtype":"success","is_error":false,"result":"","origin":{"kind":"task-notification"},"total_cost_usd":0.03}`
	var st state
	var got []float64
	for _, line := range []string{turn(1.00), turn(1.10), background, turn(1.20)} {
		for _, ev := range parse([]byte(line), &st) {
			if ev.CostUSD != nil {
				got = append(got, *ev.CostUSD)
			}
		}
	}
	if want := []float64{1.00, 1.10, 1.20}; !slices.Equal(got, want) {
		t.Errorf("costs = %v, want %v", got, want)
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
		{Kind: agent.KindTurnEnd, SessionUsage: &agent.Usage{InputTokens: 1, OutputTokens: 2, CacheReadTokens: 4, CacheWriteTokens: 3}},
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

func TestSendAfterFailedResumeNamesTheReason(t *testing.T) {
	a := fakeClaude(t, "badresume")
	if err := a.Resume(context.Background(), agent.Config{Cwd: t.TempDir()}, "x"); err != nil {
		t.Fatal(err)
	}
	var err error
	for deadline := time.Now().Add(10 * time.Second); err == nil && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
		err = a.Send(context.Background(), "hi")
	}
	if err == nil || !strings.HasPrefix(err.Error(), "write prompt: ") ||
		!strings.HasSuffix(err.Error(), ": No conversation found") {
		t.Errorf("Send error = %v, want a failed write that ends in the reason claude gave", err)
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

func TestMCPServersNotPreApproved(t *testing.T) {
	a := fakeClaude(t, "argv")
	cfg := agent.Config{
		Cwd:        t.TempDir(),
		Tools:      []string{"mcp__reef__*"},
		MCPServers: map[string]agent.MCPServer{"reef": {Command: "r"}, "mcp1": {URL: "http://x"}},
	}
	if err := a.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got := collect(t, drain(a))
	if len(got) != 1 || !strings.HasSuffix(got[0].Output, " --allowedTools mcp__reef__*") ||
		strings.Contains(got[0].Output, "mcp__mcp1__") {
		t.Errorf("events = %+v", got)
	}
}

func TestMCPConfigRemovedAtFirstLine(t *testing.T) {
	a := fakeClaude(t, "hang")
	cfg := agent.Config{Cwd: t.TempDir(), MCPServers: map[string]agent.MCPServer{"reef": {Command: "r"}}}
	if err := a.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	<-a.Events()
	if _, err := os.Stat(a.mcpPath); !os.IsNotExist(err) {
		t.Errorf("stat %s while claude runs: %v, want not exist", a.mcpPath, err)
	}
}

func TestToolsAllowed(t *testing.T) {
	a := fakeClaude(t, "argv")
	cfg := agent.Config{Cwd: t.TempDir(), Tools: []string{"Read", "Bash(git log:*)"}}
	if err := a.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got := collect(t, drain(a))
	if len(got) != 1 || !strings.Contains(got[0].Output, " --allowedTools Read,Bash(git log:*)") {
		t.Errorf("events = %+v", got)
	}
}

func TestModel(t *testing.T) {
	tests := []struct {
		name string
		give string
		want bool
	}{
		{"set", "sonnet", true},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := fakeClaude(t, "argv")
			if err := a.Start(context.Background(), agent.Config{Cwd: t.TempDir(), Model: tt.give}); err != nil {
				t.Fatal(err)
			}
			got := collect(t, drain(a))
			if len(got) != 1 || strings.Contains(got[0].Output, "--model") != tt.want ||
				tt.want && !strings.HasSuffix(got[0].Output, " --model sonnet") {
				t.Errorf("events = %+v, want --model sonnet only when set", got)
			}
		})
	}
}

func TestToolsDisallowed(t *testing.T) {
	a := fakeClaude(t, "argv")
	cfg := agent.Config{Cwd: t.TempDir(), DisallowedTools: []string{"Edit", "Write"}}
	if err := a.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got := collect(t, drain(a))
	if len(got) != 1 || !strings.Contains(got[0].Output, " --disallowedTools Edit,Write") {
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

func TestBin(t *testing.T) {
	a := fakeClaude(t, "argv")
	err := a.Start(context.Background(), agent.Config{Cwd: t.TempDir(), Bin: "/nonexistent/claude"})
	if err == nil || !strings.Contains(err.Error(), "/nonexistent/claude") {
		t.Errorf("Start error = %v, want it to name /nonexistent/claude, not the claude on PATH", err)
	}
}
