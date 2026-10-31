package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestNavigateREST(t *testing.T) {
	ts := newTestServer(t)
	for _, c := range []struct {
		method, path, body string
		want               int
		wantBody           string
	}{
		{http.MethodPost, "/bots", `{"name":"sam","backend":"claude"}`, http.StatusCreated, ""},
		{http.MethodPost, "/spaces", `{"name":"talk about sam","botId":2,"kind":"conversation"}`, http.StatusCreated, ""},
		{http.MethodPut, "/pins/bot/1", "", http.StatusNoContent, ""},
		{http.MethodPut, "/pins/chat/1", "", http.StatusNoContent, ""},
		{http.MethodPut, "/pins/space/1", "", http.StatusNotFound, ""},
		{http.MethodPut, "/pins/team/1", "", http.StatusBadRequest, ""},
		{http.MethodGet, "/pins", "", http.StatusOK, `"kind":"chat","id":1,"title":"talk about sam"`},
		{http.MethodDelete, "/pins/chat/1", "", http.StatusNoContent, ""},
		{http.MethodGet, "/find?q=SAM", "", http.StatusOK, `[{"kind":"bot","id":2,"title":"sam"},{"kind":"chat","id":1,"title":"talk about sam"}]`},
		{http.MethodGet, "/find", "", http.StatusBadRequest, ""},
		{http.MethodGet, "/bots/2/conversations?limit=1&before=1", "", http.StatusOK, `[]`},
		{http.MethodGet, "/messages?space=1&limit=2", "", http.StatusOK, `[]`},
	} {
		status, body := do(t, c.method, ts.URL+c.path, c.body)
		if status != c.want || !strings.Contains(body, c.wantBody) {
			t.Errorf("%s %s = %d %s, want %d %s", c.method, c.path, status, body, c.want, c.wantBody)
		}
	}
}
