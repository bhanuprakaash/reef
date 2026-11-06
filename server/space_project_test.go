package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/bhanuprakaash/reef/store"
)

func spaceNames(t *testing.T, url string) []string {
	t.Helper()
	status, body := do(t, http.MethodGet, url, "")
	var spaces []store.SpacePreview
	if status != 200 || json.Unmarshal([]byte(body), &spaces) != nil {
		t.Fatalf("GET %s = %d %s, want 200 and Spaces", url, status, body)
	}
	var names []string
	for _, c := range spaces {
		names = append(names, c.Name)
	}
	return names
}

func TestSpaceProjects_GlobalSpacesAndFilter(t *testing.T) {
	ts, st := newEmbedServer(t, nil)
	bot, err := st.CreateBot(context.Background(), store.Bot{Name: "ada", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, name := range []string{"shop", "blog"} {
		if status, body := do(t, http.MethodPost, ts.URL+"/projects", `{"name":"`+name+`","path":"`+dir+`"}`); status != 201 {
			t.Fatalf("create project = %d %s", status, body)
		}
	}
	for _, body := range []string{
		`{"botId":%d,"kind":"conversation","name":"global chat"}`,
		`{"botId":%d,"name":"global space"}`,
		`{"botId":%d,"kind":"conversation","name":"shop chat","projectId":1}`,
	} {
		body = fmt.Sprintf(body, bot.ID)
		if status, got := do(t, http.MethodPost, ts.URL+"/spaces", body); status != 201 {
			t.Fatalf("POST /spaces %s = %d %s, want 201", body, status, got)
		}
	}

	if got, want := spaceNames(t, ts.URL+"/spaces"), []string{"global chat", "global space", "shop chat"}; !reflect.DeepEqual(got, want) {
		t.Errorf("GET /spaces = %v, want %v", got, want)
	}
	if got, want := spaceNames(t, ts.URL+"/spaces?project=1"), []string{"shop chat"}; !reflect.DeepEqual(got, want) {
		t.Errorf("GET /spaces?project=1 = %v, want %v", got, want)
	}

	if status, body := do(t, http.MethodPut, ts.URL+"/spaces/1/projects/1", ""); status != 200 ||
		body[:15] != `[{"id":1,"name"` {
		t.Errorf("PUT /spaces/1/projects/1 = %d %s, want 200 and project 1", status, body)
	}
	if status, body := do(t, http.MethodPut, ts.URL+"/spaces/1/projects/9", ""); status != 404 ||
		body != `{"error":"project 9: not found"}` {
		t.Errorf("PUT /spaces/1/projects/9 = %d %s, want 404", status, body)
	}
	if got, want := spaceNames(t, ts.URL+"/spaces?project=1"), []string{"global chat", "shop chat"}; !reflect.DeepEqual(got, want) {
		t.Errorf("GET /spaces?project=1 after add = %v, want %v", got, want)
	}
	if status, body := do(t, http.MethodDelete, ts.URL+"/spaces/1/projects/1", ""); status != 200 || body != "[]" {
		t.Errorf("DELETE /spaces/1/projects/1 = %d %s, want 200 []", status, body)
	}
	if got, want := spaceNames(t, ts.URL+"/spaces?project=1"), []string{"shop chat"}; !reflect.DeepEqual(got, want) {
		t.Errorf("GET /spaces?project=1 after remove = %v, want %v", got, want)
	}
}

func TestDeleteProject_KeepsASpaceWithOtherProjects(t *testing.T) {
	ts, st := newEmbedServer(t, nil)
	bot, err := st.CreateBot(context.Background(), store.Bot{Name: "ada", Backend: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, name := range []string{"shop", "blog"} {
		do(t, http.MethodPost, ts.URL+"/projects", `{"name":"`+name+`","path":"`+dir+`"}`)
	}
	for _, name := range []string{"only shop", "both"} {
		do(t, http.MethodPost, ts.URL+"/spaces",
			fmt.Sprintf(`{"botId":%d,"kind":"conversation","name":"%s","projectId":1}`, bot.ID, name))
	}
	do(t, http.MethodPut, ts.URL+"/spaces/2/projects/2", "")

	if status, body := do(t, http.MethodDelete, ts.URL+"/projects/1", ""); status != 204 {
		t.Fatalf("DELETE /projects/1 = %d %s, want 204", status, body)
	}

	if got, want := spaceNames(t, ts.URL+"/spaces"), []string{"only shop", "both"}; !reflect.DeepEqual(got, want) {
		t.Errorf("GET /spaces after delete = %v, want %v", got, want)
	}
	if got, want := spaceNames(t, ts.URL+"/spaces?project=2"), []string{"both"}; !reflect.DeepEqual(got, want) {
		t.Errorf("GET /spaces?project=2 = %v, want %v", got, want)
	}
	if status, body := do(t, http.MethodGet, ts.URL+"/spaces/1/projects", ""); status != 200 || body != "[]" {
		t.Errorf("GET /spaces/1/projects = %d %s, want 200 []", status, body)
	}
}
