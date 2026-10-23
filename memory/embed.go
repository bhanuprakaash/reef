package memory

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
)

const (
	ProviderNone   = "none"
	ProviderOpenAI = "openai"
	ProviderVoyage = "voyage"
)

const (
	_openAIURL   = "https://api.openai.com/v1/embeddings"
	_openAIModel = "text-embedding-3-small"
	_voyageURL   = "https://api.voyageai.com/v1/embeddings"
	_voyageModel = "voyage-3-lite"
)

// Embedder turns text into a vector. A nil vector with a nil error means the
// embedder produces none.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// EmbedConfig names the embedding provider and its key.
type EmbedConfig struct{ Provider, Key string }

type noneEmbedder struct{}

func (noneEmbedder) Embed(context.Context, string) ([]float32, error) { return nil, nil }

type openAIEmbedder struct {
	hc       *http.Client
	url, key string
}

func (e openAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return postEmbedding(ctx, e.hc, ProviderOpenAI, e.url, e.key,
		struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}{_openAIModel, text})
}

type voyageEmbedder struct {
	hc       *http.Client
	url, key string
}

func (e voyageEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return postEmbedding(ctx, e.hc, ProviderVoyage, e.url, e.key,
		struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}{_voyageModel, []string{text}})
}

// NewEmbedder returns the embedder c names. An empty key or provider gives the
// none embedder; an unknown provider is an error. A nil hc means
// http.DefaultClient.
func NewEmbedder(c EmbedConfig, hc *http.Client) (Embedder, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	switch c.Provider {
	case "", ProviderNone:
	case ProviderOpenAI:
		if c.Key != "" {
			return openAIEmbedder{hc: hc, url: _openAIURL, key: c.Key}, nil
		}
	case ProviderVoyage:
		if c.Key != "" {
			return voyageEmbedder{hc: hc, url: _voyageURL, key: c.Key}, nil
		}
	default:
		return nil, fmt.Errorf("unknown embedding provider %q", c.Provider)
	}
	return noneEmbedder{}, nil
}

// postEmbedding sends body to url and returns data[0].embedding, the response
// shape OpenAI and Voyage share. Errors carry the status code, never the key.
func postEmbedding(
	ctx context.Context, hc *http.Client, name, url, key string, body any,
) ([]float32, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", name, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", name, err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s embeddings: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s embeddings: status %d", name, resp.StatusCode)
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", name, err)
	}
	if len(out.Data) == 0 || len(out.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("%s embeddings: no embedding in response", name)
	}
	return out.Data[0].Embedding, nil
}

// Backfill embeds every item that has no vector and returns how many it
// filled. It stops at the first embed error.
func (s *Store) Backfill(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, body, why FROM memory WHERE vector IS NULL ORDER BY id`)
	if err != nil {
		return 0, fmt.Errorf("query unembedded: %w", err)
	}
	defer rows.Close()
	type pending struct {
		id        int64
		body, why string
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.body, &p.why); err != nil {
			return 0, fmt.Errorf("scan unembedded: %w", err)
		}
		todo = append(todo, p)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("scan unembedded: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close unembedded: %w", err)
	}
	filled := 0
	for _, p := range todo {
		ok, err := s.embedItem(ctx, p.id, embedText(p.body, p.why))
		if err != nil {
			return filled, fmt.Errorf("embed %d: %w", p.id, err)
		}
		if ok {
			filled++
		}
	}
	return filled, nil
}

// Nearest returns up to k items that pass f and have a vector, closest to
// query first. Score is the cosine similarity.
func (s *Store) Nearest(ctx context.Context, query string, f Filter, k int) ([]Hit, error) {
	if k <= 0 || len(f.Scopes) == 0 {
		return nil, nil
	}
	q, err := s.embed.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if q == nil {
		return nil, nil
	}
	where, args := f.where()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, status, vector FROM memory WHERE vector IS NOT NULL AND `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query vectors: %w", err)
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var (
			h    Hit
			blob []byte
		)
		if err := rows.Scan(&h.ID, &h.Status, &blob); err != nil {
			return nil, fmt.Errorf("scan vector: %w", err)
		}
		v := decodeVector(blob)
		if len(v) != len(q) {
			continue
		}
		h.Score = cosine(q, v)
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan vectors: %w", err)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].ID < hits[j].ID
	})
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits, nil
}

// embedItem stores the vector for item id. It reports false when the embedder
// makes none.
func (s *Store) embedItem(ctx context.Context, id int64, text string) (bool, error) {
	v, err := s.embed.Embed(ctx, text)
	if err != nil || v == nil {
		return false, err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE memory SET vector = ? WHERE id = ?`, encodeVector(v), id); err != nil {
		return false, fmt.Errorf("store vector: %w", err)
	}
	return true, nil
}

func embedText(body, why string) string {
	if why == "" {
		return body
	}
	return body + "\n" + why
}

func encodeVector(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return b
}

func decodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
