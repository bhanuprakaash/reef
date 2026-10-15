package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestProfileRoundTrip(t *testing.T) {
	ts := newTestServer(t)

	status, body := do(t, http.MethodGet, ts.URL+"/profile", "")
	if status != 404 || body != `{"error":"profile: not found"}` {
		t.Fatalf("before a name = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/profile", `{"name":" "}`)
	if status != 400 || body != `{"error":"invalid: profile name is required"}` {
		t.Fatalf("blank name = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/profile", `{"name":" Ada "}`)
	if status != 200 || !strings.HasPrefix(body, `{"name":"Ada","occupation":"","style":[],"updatedAt":"`) {
		t.Fatalf("put profile = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/profile",
		`{"name":"Ada","occupation":" Developer ","style":["Short answers"," ","Tests first"]}`)
	if status != 200 {
		t.Fatalf("put occupation and style = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/profile", "")
	want := `{"name":"Ada","occupation":"Developer","style":["Short answers","Tests first"],"updatedAt":"`
	if status != 200 || !strings.HasPrefix(body, want) {
		t.Fatalf("get profile = %d %s", status, body)
	}

	status, body = do(t, http.MethodDelete, ts.URL+"/profile", "")
	if status != 405 || body != `{"error":"method not allowed"}` {
		t.Fatalf("wrong method = %d %s", status, body)
	}
}
