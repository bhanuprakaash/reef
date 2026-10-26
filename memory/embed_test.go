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
	"time"
)

type fakeEmbedder struct {
	vectors map[string][]float32
	err     error
}

func (f *fakeEmbedder) MinCosine() float64 { return _minCosine }

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
		{"gemini without key", EmbedConfig{Provider: "gemini"}},
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
	if got, want := ids(hits), []int64{2, 4}; !reflect.DeepEqual(got, want) {
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

func TestRemember_ReportsEmbedFailure(t *testing.T) {
	s, db := openTest(t)
	s.embed = &fakeEmbedder{err: errors.New("openai embeddings: status 429")}
	res, err := s.Remember(context.Background(), writer(), Note{Kind: KindFact, Body: "uses sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if res.EmbedErr == nil || res.EmbedErr.Error() != "openai embeddings: status 429" {
		t.Fatalf("EmbedErr = %v, want the embedder's error", res.EmbedErr)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM memory`); n != 1 {
		t.Fatalf("%d items stored, want 1", n)
	}

	s.embed = noneEmbedder{}
	res, err = s.Remember(context.Background(), writer(), Note{Kind: KindFact, Body: "uses sqlite too"})
	if err != nil || res.EmbedErr != nil {
		t.Fatalf("none embedder: err = %v, EmbedErr = %v; want both nil", err, res.EmbedErr)
	}
}

func serveStatuses(t *testing.T, retryAfter string, statuses ...int) (*openAIEmbedder, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := statuses[min(calls, len(statuses)-1)]
		calls++
		if status == 429 {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(status)
		if status == 200 {
			_, _ = io.WriteString(w, `{"data":[{"embedding":[1]}]}`)
		}
	}))
	t.Cleanup(srv.Close)
	return &openAIEmbedder{hc: srv.Client(), url: srv.URL, key: "k"}, &calls
}

func TestBackfill_WaitsOutOne429PerItem(t *testing.T) {
	tests := []struct {
		name       string
		statuses   []int
		wantFilled int
		wantCalls  int
		wantErr    string
	}{
		{"then ok", []int{429, 200}, 1, 2, ""},
		{"twice", []int{429, 429, 200}, 0, 2, "embed 1: openai embeddings: status 429"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, db := openTest(t)
			remember(t, s, writer(), "uses sqlite", "")
			e, calls := serveStatuses(t, "1", tt.statuses...)
			s.embed = e

			start := time.Now()
			n, err := s.Backfill(context.Background())
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if n != tt.wantFilled || *calls != tt.wantCalls {
				t.Fatalf("filled %d with %d requests, want %d with %d", n, *calls, tt.wantFilled, tt.wantCalls)
			}
			if waited := time.Since(start); waited < time.Second {
				t.Fatalf("Backfill returned after %v, want a wait of the 1 second Retry-After", waited)
			}
			if got := count(t, db, `SELECT COUNT(*) FROM memory WHERE vector IS NOT NULL`); got != tt.wantFilled {
				t.Fatalf("%d vectors stored, want %d", got, tt.wantFilled)
			}
		})
	}
}

func TestBackfill_CancelEndsTheWait(t *testing.T) {
	s, _ := openTest(t)
	remember(t, s, writer(), "uses sqlite", "")
	s.embed, _ = serveStatuses(t, "60", 429)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := s.Backfill(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if waited := time.Since(start); waited > time.Second {
		t.Fatalf("Backfill waited %v after cancel", waited)
	}
}

func TestBackfill_SkipsRejectedItems(t *testing.T) {
	s, db := openTest(t)
	s.embed = &fakeEmbedder{err: errors.New("down")}
	id := remember(t, s, writer(), "uses sqlite", "")
	if _, err := s.db.Exec(`UPDATE memory SET status = ? WHERE id = ?`, StatusRejected, id); err != nil {
		t.Fatal(err)
	}
	s.embed = &fakeEmbedder{vectors: map[string][]float32{"uses sqlite": {1, 0}}}

	if n, err := s.Backfill(context.Background()); err != nil || n != 0 {
		t.Fatalf("Backfill = %d, %v; want 0, nil", n, err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM memory WHERE vector IS NOT NULL`); n != 0 {
		t.Fatalf("%d vectors stored, want 0", n)
	}
}

func TestRecall_NeverWaitsOut429(t *testing.T) {
	s, _ := openTest(t)
	remember(t, s, writer(), "uses sqlite", "")
	e, calls := serveStatuses(t, "3", 429, 200)
	s.embed = e

	start := time.Now()
	hits, err := s.Recall(context.Background(), Query{Scopes: []Ref{writer().Scope}, Text: "sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if got := hitIDs(hits); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("hits = %v, want [1] from words", got)
	}
	if waited := time.Since(start); waited > 100*time.Millisecond {
		t.Fatalf("Recall took %v, want no wait on a 429", waited)
	}
	if *calls != 1 {
		t.Fatalf("%d requests, want 1", *calls)
	}
}

func TestRemember_NeverWaitsOut429(t *testing.T) {
	s, _ := openTest(t)
	s.embed, _ = serveStatuses(t, "3", 429, 200)

	start := time.Now()
	res, err := s.Remember(context.Background(), writer(), Note{Kind: KindFact, Body: "uses sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if res.EmbedErr == nil || res.EmbedErr.Error() != "openai embeddings: status 429" {
		t.Fatalf("EmbedErr = %v, want status 429", res.EmbedErr)
	}
	if waited := time.Since(start); waited > 100*time.Millisecond {
		t.Fatalf("Remember took %v, want no wait on a 429", waited)
	}
}

func TestRetryAfter(t *testing.T) {
	tests := []struct {
		give string
		want time.Duration
	}{
		{"", time.Second},
		{"soon", time.Second},
		{"-5", time.Second},
		{"0", time.Second},
		{"3", 3 * time.Second},
		{"60", 60 * time.Second},
		{"61", 60 * time.Second},
		{"9999999999", 60 * time.Second},
	}
	for _, tt := range tests {
		if got := retryAfter(tt.give); got != tt.want {
			t.Errorf("retryAfter(%q) = %v, want %v", tt.give, got, tt.want)
		}
	}
}

func TestEmbedders_ErrorNamesVendorCodeNotMessage(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"gemini status", `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"key sk-secret over quota"}}`,
			"openai embeddings: status 400 RESOURCE_EXHAUSTED"},
		{"openai code", `{"error":{"code":"insufficient_quota","message":"key sk-secret over quota"}}`,
			"openai embeddings: status 400 insufficient_quota"},
		{"message only", `{"error":{"message":"key sk-secret"}}`, "openai embeddings: status 400"},
		{"not json", `key sk-secret`, "openai embeddings: status 400"},
		{"code with spaces", `{"error":{"code":"key sk-secret"}}`, "openai embeddings: status 400"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			_, err := openAIEmbedder{hc: srv.Client(), url: srv.URL, key: "sk-secret"}.Embed(context.Background(), "x")
			if err == nil || err.Error() != tt.want {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEmbedders_VendorShapes(t *testing.T) {
	tests := []struct {
		name       string
		give       func(url string, hc *http.Client) Embedder
		path       string
		authHeader string
		wantAuth   string
		wantBody   string
		response   string
	}{
		{
			name: "openai",
			give: func(url string, hc *http.Client) Embedder {
				return openAIEmbedder{hc: hc, url: url, key: "sk-secret"}
			},
			path:       "/v1/embeddings",
			authHeader: "Authorization",
			wantAuth:   "Bearer sk-secret",
			wantBody:   `{"model":"text-embedding-3-small","input":"hello"}`,
			response: `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.5,-0.25,2]}],` +
				`"model":"text-embedding-3-small","usage":{"prompt_tokens":1,"total_tokens":1}}`,
		},
		{
			name: "voyage",
			give: func(url string, hc *http.Client) Embedder {
				return voyageEmbedder{hc: hc, url: url, key: "sk-secret"}
			},
			path:       "/v1/embeddings",
			authHeader: "Authorization",
			wantAuth:   "Bearer sk-secret",
			wantBody:   `{"model":"voyage-3-lite","input":["hello"]}`,
			response: `{"object":"list","data":[{"object":"embedding","embedding":[0.5,-0.25,2],"index":0}],` +
				`"model":"voyage-3-lite","usage":{"total_tokens":1}}`,
		},
		{
			name: "gemini",
			give: func(url string, hc *http.Client) Embedder {
				return geminiEmbedder{hc: hc, url: url, key: "sk-secret"}
			},
			path:       "/v1beta/models/gemini-embedding-001:embedContent",
			authHeader: "x-goog-api-key",
			wantAuth:   "sk-secret",
			wantBody:   `{"content":{"parts":[{"text":"hello"}]},"outputDimensionality":768}`,
			response:   `{"embedding":{"values":[0.5,-0.25,2]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var method, path, auth, ctype, body string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				method, path, auth, ctype, body = r.Method, r.URL.Path, r.Header.Get(tt.authHeader),
					r.Header.Get("Content-Type"), string(b)
				_, _ = io.WriteString(w, tt.response)
			}))
			defer srv.Close()

			got, err := tt.give(srv.URL+tt.path, srv.Client()).Embed(context.Background(), "hello")
			if err != nil {
				t.Fatal(err)
			}
			if want := []float32{0.5, -0.25, 2}; !reflect.DeepEqual(got, want) {
				t.Fatalf("vector = %v, want %v", got, want)
			}
			if method != http.MethodPost || path != tt.path {
				t.Fatalf("request = %s %s", method, path)
			}
			if auth != tt.wantAuth || ctype != "application/json" {
				t.Fatalf("headers: %s %q, Content-Type %q", tt.authHeader, auth, ctype)
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
		"gemini": geminiEmbedder{hc: srv.Client(), url: srv.URL, key: "sk-secret"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := e.Embed(context.Background(), "hello")
			if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "sk-secret") {
				t.Fatalf("err = %v, want one with 401 and no key", err)
			}
		})
	}
}

func TestNewEmbedder_DefaultClientTimesOut(t *testing.T) {
	e, err := NewEmbedder(EmbedConfig{Provider: ProviderOpenAI, Key: "k"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := e.(openAIEmbedder).hc.Timeout, 10*time.Second; got != want {
		t.Fatalf("timeout = %v, want %v", got, want)
	}
	e, err = NewEmbedder(EmbedConfig{Provider: ProviderVoyage, Key: "k"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := e.(voyageEmbedder).hc.Timeout, 10*time.Second; got != want {
		t.Fatalf("voyage timeout = %v, want %v", got, want)
	}
	e, err = NewEmbedder(EmbedConfig{Provider: ProviderGemini, Key: "k"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := e.(geminiEmbedder).hc.Timeout, 10*time.Second; got != want {
		t.Fatalf("gemini timeout = %v, want %v", got, want)
	}
}

func TestNewEmbedder_UnknownProvider(t *testing.T) {
	if _, err := NewEmbedder(EmbedConfig{Provider: "cohere", Key: "k"}, nil); err == nil {
		t.Fatal("want an error for an unknown provider")
	}
}

func TestEmbedders_EmptyEmbeddingFails(t *testing.T) {
	for name, tt := range map[string]struct {
		body string
		give func(url string, hc *http.Client) Embedder
	}{
		"openai": {`{"data":[{"embedding":[]}]}`, func(url string, hc *http.Client) Embedder {
			return openAIEmbedder{hc: hc, url: url, key: "k"}
		}},
		"voyage": {`{"data":[]}`, func(url string, hc *http.Client) Embedder {
			return voyageEmbedder{hc: hc, url: url, key: "k"}
		}},
		"gemini": {`{"embedding":{"values":[]}}`, func(url string, hc *http.Client) Embedder {
			return geminiEmbedder{hc: hc, url: url, key: "k"}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			_, err := tt.give(srv.URL, srv.Client()).Embed(context.Background(), "hello")
			if want := name + " embeddings: no embedding in response"; err == nil || err.Error() != want {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
}

func TestGeminiURL(t *testing.T) {
	if want := "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:embedContent"; _geminiURL != want {
		t.Fatalf("_geminiURL = %q, want %q", _geminiURL, want)
	}
}
