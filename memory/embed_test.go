package memory

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type fakeEmbedder struct {
	vectors map[string][]float32
	err     error
}

func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.vectors[text], nil
}

func remember(t *testing.T, s *Store, w Writer, body, why string) int64 {
	t.Helper()
	res, err := s.Remember(context.Background(), w, Note{Kind: KindFact, Body: body, Why: why})
	if err != nil {
		t.Fatal(err)
	}
	return res.Item.ID
}

func TestNone_MakesNoHTTPCall(t *testing.T) {
	failing := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected HTTP call to %s", r.URL)
		return nil, errors.New("blocked")
	})}
	tests := []struct {
		name string
		give EmbedConfig
	}{
		{"empty config", EmbedConfig{}},
		{"provider none with key", EmbedConfig{Provider: "none", Key: "k"}},
		{"openai without key", EmbedConfig{Provider: "openai"}},
		{"voyage without key", EmbedConfig{Provider: "voyage"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewEmbedder(tt.give, failing)
			if err != nil {
				t.Fatal(err)
			}
			s, db := openTest(t)
			s.embed = e
			ctx := context.Background()
			remember(t, s, writer(), "uses sqlite", "local first")
			if n, err := s.Backfill(ctx); err != nil || n != 0 {
				t.Fatalf("Backfill = %d, %v; want 0, nil", n, err)
			}
			hits, err := s.Nearest(ctx, "sqlite", Filter{Scopes: []Ref{writer().Scope}}, 5)
			if err != nil || hits != nil {
				t.Fatalf("Nearest = %v, %v; want nil, nil", hits, err)
			}
			hits, err = s.Search(ctx, "sqlite", Filter{Scopes: []Ref{writer().Scope}})
			if err != nil || len(hits) != 1 || hits[0].ID != 1 {
				t.Fatalf("Search = %v, %v; want id 1", hits, err)
			}
			if n := count(t, db, `SELECT COUNT(*) FROM memory WHERE vector IS NOT NULL`); n != 0 {
				t.Fatalf("%d vectors stored, want 0", n)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNearest_OrdersByCosine(t *testing.T) {
	fake := &fakeEmbedder{vectors: map[string][]float32{
		"north":        {1, 0},
		"north-east":   {1, 1},
		"east":         {0, 1},
		"south":        {-1, 0},
		"elsewhere":    {1, 0},
		"retired":      {1, 0},
		"pending":      {1, 0},
		"want north":   {1, 0.1},
		"want nothing": nil,
	}}
	s, _ := openTest(t)
	s.embed = fake
	ctx := context.Background()
	w := writer()
	other := w
	other.Scope = Ref{Kind: ScopeWorkItem, ID: "8"}

	remember(t, s, w, "east", "")
	remember(t, s, w, "north", "")
	remember(t, s, w, "south", "")
	remember(t, s, w, "north-east", "")
	remember(t, s, other, "elsewhere", "")
	old := remember(t, s, w, "retired", "")
	pending := remember(t, s, w, "pending", "")
	if err := s.Retire(ctx, old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE memory SET status = ? WHERE id = ?`, StatusCandidate, pending); err != nil {
		t.Fatal(err)
	}

	f := Filter{Scopes: []Ref{w.Scope}}
	hits, err := s.Nearest(ctx, "want north", f, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(hits), []int64{2, 4, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	if hits[0].Score < 0.99 || hits[0].Status != StatusAccepted {
		t.Fatalf("top hit = %+v", hits[0])
	}

	f.IncludeRetired = true
	hits, err = s.Nearest(ctx, "want north", f, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(hits), []int64{2, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("with retired: ids = %v, want %v", got, want)
	}
	if hits[1].Status != StatusRetired {
		t.Fatalf("status = %q, want retired", hits[1].Status)
	}

	hits, err = s.Nearest(ctx, "want nothing", f, 3)
	if err != nil || hits != nil {
		t.Fatalf("no query vector: %v, %v; want nil, nil", hits, err)
	}
	hits, err = s.Nearest(ctx, "want north", Filter{}, 3)
	if err != nil || hits != nil {
		t.Fatalf("no scopes: %v, %v; want nil, nil", hits, err)
	}
}

func ids(hits []Match) []int64 {
	out := make([]int64, len(hits))
	for i, h := range hits {
		out[i] = h.ID
	}
	return out
}

func TestRemember_StoresWhenEmbedderFailsThenBackfills(t *testing.T) {
	fake := &fakeEmbedder{err: errors.New("down")}
	s, db := openTest(t)
	s.embed = fake
	ctx := context.Background()

	id := remember(t, s, writer(), "uses sqlite", "local first")
	if _, err := s.Read(ctx, id); err != nil {
		t.Fatalf("item not stored: %v", err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM memory WHERE vector IS NULL`); n != 1 {
		t.Fatalf("%d items without vector, want 1", n)
	}
	if n, err := s.Backfill(ctx); err == nil || n != 0 {
		t.Fatalf("Backfill while down = %d, %v; want 0 and an error", n, err)
	}

	fake.err = nil
	fake.vectors = map[string][]float32{"uses sqlite\nlocal first": {0.5, -2}}
	if n, err := s.Backfill(ctx); err != nil || n != 1 {
		t.Fatalf("Backfill = %d, %v; want 1, nil", n, err)
	}
	var blob []byte
	if err := db.QueryRow(`SELECT vector FROM memory WHERE id = ?`, id).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	if want := []byte{0, 0, 0, 0x3f, 0, 0, 0, 0xc0}; !reflect.DeepEqual(blob, want) {
		t.Fatalf("vector bytes = %v, want %v", blob, want)
	}
	if n, err := s.Backfill(ctx); err != nil || n != 0 {
		t.Fatalf("second Backfill = %d, %v; want 0, nil", n, err)
	}
}

func TestEmbedders_VendorShapes(t *testing.T) {
	tests := []struct {
		name     string
		give     func(url string, hc *http.Client) Embedder
		wantBody string
		response string
	}{
		{
			name: "openai",
			give: func(url string, hc *http.Client) Embedder {
				return openAIEmbedder{hc: hc, url: url, key: "sk-secret"}
			},
			wantBody: `{"model":"text-embedding-3-small","input":"hello"}`,
			response: `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.5,-0.25,2]}],` +
				`"model":"text-embedding-3-small","usage":{"prompt_tokens":1,"total_tokens":1}}`,
		},
		{
			name: "voyage",
			give: func(url string, hc *http.Client) Embedder {
				return voyageEmbedder{hc: hc, url: url, key: "sk-secret"}
			},
			wantBody: `{"model":"voyage-3-lite","input":["hello"]}`,
			response: `{"object":"list","data":[{"object":"embedding","embedding":[0.5,-0.25,2],"index":0}],` +
				`"model":"voyage-3-lite","usage":{"total_tokens":1}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var method, path, auth, ctype, body string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				method, path, auth, ctype, body = r.Method, r.URL.Path, r.Header.Get("Authorization"),
					r.Header.Get("Content-Type"), string(b)
				_, _ = io.WriteString(w, tt.response)
			}))
			defer srv.Close()

			got, err := tt.give(srv.URL+"/v1/embeddings", srv.Client()).Embed(context.Background(), "hello")
			if err != nil {
				t.Fatal(err)
			}
			if want := []float32{0.5, -0.25, 2}; !reflect.DeepEqual(got, want) {
				t.Fatalf("vector = %v, want %v", got, want)
			}
			if method != http.MethodPost || path != "/v1/embeddings" {
				t.Fatalf("request = %s %s", method, path)
			}
			if auth != "Bearer sk-secret" || ctype != "application/json" {
				t.Fatalf("headers: Authorization %q, Content-Type %q", auth, ctype)
			}
			if body != tt.wantBody {
				t.Fatalf("body = %s, want %s", body, tt.wantBody)
			}
		})
	}
}

func TestEmbedders_ErrorsNameStatusNotKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Incorrect API key provided: sk-secret"}}`)
	}))
	defer srv.Close()
	for name, e := range map[string]Embedder{
		"openai": openAIEmbedder{hc: srv.Client(), url: srv.URL, key: "sk-secret"},
		"voyage": voyageEmbedder{hc: srv.Client(), url: srv.URL, key: "sk-secret"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := e.Embed(context.Background(), "hello")
			if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "sk-secret") {
				t.Fatalf("err = %v, want one with 401 and no key", err)
			}
		})
	}
}

func TestNewEmbedder_UnknownProvider(t *testing.T) {
	if _, err := NewEmbedder(EmbedConfig{Provider: "cohere", Key: "k"}, nil); err == nil {
		t.Fatal("want an error for an unknown provider")
	}
}
