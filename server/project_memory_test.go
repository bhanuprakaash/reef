package server

import (
	"net/http"
	"testing"
)

func TestProjectMemoryRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	do(t, http.MethodPost, ts.URL+"/projects", `{"name":"reef","path":"/tmp"}`)

	status, body := do(t, http.MethodGet, ts.URL+"/projects/1/memory", "")
	if status != 200 || body != `{"projectId":1,"body":""}` {
		t.Fatalf("empty memory = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/projects/1/memory", `{"body":"use sessions"}`)
	if status != 200 || body != `{"projectId":1,"body":"use sessions"}` {
		t.Fatalf("put memory = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/projects/1/memory", "")
	if status != 200 || body != `{"projectId":1,"body":"use sessions"}` {
		t.Fatalf("get memory = %d %s", status, body)
	}

	status, body = do(t, http.MethodGet, ts.URL+"/projects/9/memory", "")
	if status != 404 || body != `{"error":"project 9: not found"}` {
		t.Fatalf("unknown project = %d %s", status, body)
	}

	status, body = do(t, http.MethodPut, ts.URL+"/projects/9/memory", `{"body":"x"}`)
	if status != 404 || body != `{"error":"project 9: not found"}` {
		t.Fatalf("put on unknown project = %d %s", status, body)
	}

	status, body = do(t, http.MethodPatch, ts.URL+"/projects/1/memory", "")
	if status != 405 || body != `{"error":"method not allowed"}` {
		t.Fatalf("wrong method = %d %s", status, body)
	}
}
