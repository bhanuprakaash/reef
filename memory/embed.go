package memory

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ProviderNone   = "none"
	ProviderOpenAI = "openai"
	ProviderVoyage = "voyage"
	ProviderGemini = "gemini"
)

const (
	_openAIURL   = "https://api.openai.com/v1/embeddings"
	_openAIModel = "text-embedding-3-small"
	_voyageURL   = "https://api.voyageai.com/v1/embeddings"
	_voyageModel = "voyage-3-lite"
	_geminiURL   = "https://generativelanguage.googleapis.com/v1beta/models/" +
		"gemini-embedding-001:embedContent"
	_geminiDims = 768
)

const (
	_minCosine       = 0.3
	_geminiMinCosine = 0.6
	_embedTimeout    = 10 * time.Second

	_maxRetryAfter = 60 * time.Second
	_codeChars     = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-"
)

var _errEmbedQuery = errors.New("embed query")

// Embedder turns text into a vector. A nil vector with a nil error means none. MinCosine is
// the similarity under which a vector match is unrelated; each model spreads scores
// differently.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	MinCosine() float64
}

// EmbedConfig names the embedding provider and its key.
type EmbedConfig struct{ Provider, Key string }

type noneEmbedder struct{}

func (noneEmbedder) Embed(context.Context, string) ([]float32, error) { return nil, nil }

func (noneEmbedder) MinCosine() float64 { return 0 }

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

func (openAIEmbedder) MinCosine() float64 { return _minCosine }

func (e voyageEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return postEmbedding(ctx, e.hc, ProviderVoyage, e.url, e.key,
		struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}{_voyageModel, []string{text}})
}

type geminiEmbedder struct {
	hc       *http.Client
	url, key string
}

func (voyageEmbedder) MinCosine() float64 { return _minCosine }

func (geminiEmbedder) MinCosine() float64 { return _geminiMinCosine }

func (e geminiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Parts []part `json:"parts"`
	}
	var out struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}
	body := struct {
		Content content `json:"content"`
		Dims    int     `json:"outputDimensionality"`
	}{content{[]part{{text}}}, _geminiDims}
	err := postJSON(ctx, e.hc, ProviderGemini, e.url, "x-goog-api-key", e.key, body, &out)
	if err != nil {
		return nil, err
	}
	if len(out.Embedding.Values) == 0 {
		return nil, fmt.Errorf("%s embeddings: no embedding in response", ProviderGemini)
	}
	return out.Embedding.Values, nil
}

// NewEmbedder returns the embedder c names. An empty key or provider gives the none
// embedder; an unknown provider is an error. A nil hc means a client that gives up after
// 10 seconds.
func NewEmbedder(c EmbedConfig, hc *http.Client) (Embedder, error) {
	if hc == nil {
		hc = &http.Client{Timeout: _embedTimeout}
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
	case ProviderGemini:
		if c.Key != "" {
			return geminiEmbedder{hc: hc, url: _geminiURL, key: c.Key}, nil
		}
	default:
		return nil, fmt.Errorf("unknown embedding provider %q", c.Provider)
	}
	return noneEmbedder{}, nil
}

func postEmbedding(
	ctx context.Context, hc *http.Client, name, url, key string, body any,
) ([]float32, error) {
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := postJSON(ctx, hc, name, url, "Authorization", "Bearer "+key, body, &out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 || len(out.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("%s embeddings: no embedding in response", name)
	}
	return out.Data[0].Embedding, nil
}

// Errors carry the status code and the vendor's error code, never the key or the vendor's
// message. A 429 is retried once after its Retry-After.
func postJSON(
	ctx context.Context, hc *http.Client, name, url, authHeader, auth string, body, out any,
) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", name, err)
	}
	send := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("build %s request: %w", name, err)
		}
		req.Header.Set(authHeader, auth)
		req.Header.Set("Content-Type", "application/json")
		resp, err := hc.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s embeddings: %w", name, err)
		}
		return resp, nil
	}
	resp, err := send()
	if err == nil && resp.StatusCode == http.StatusTooManyRequests {
		wait := retryAfter(resp.Header.Get("Retry-After"))
		resp.Body.Close()
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return fmt.Errorf("%s embeddings: %w", name, ctx.Err())
		}
		resp, err = send()
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := fmt.Sprintf("%s embeddings: status %d", name, resp.StatusCode)
		if code := vendorCode(resp.Body); code != "" {
			msg += " " + code
		}
		return errors.New(msg)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s response: %w", name, err)
	}
	return nil
}

// retryAfter reads a Retry-After in seconds: 1 second when it is missing or not a
// non-negative integer, at most 60.
func retryAfter(h string) time.Duration {
	n, err := strconv.Atoi(h)
	if err != nil || n < 0 {
		n = 1
	}
	return min(time.Duration(n)*time.Second, _maxRetryAfter)
}

// vendorCode returns the error code token of a vendor error body, such as
// RESOURCE_EXHAUSTED or insufficient_quota, or "" when there is none. It never returns
// the message.
func vendorCode(r io.Reader) string {
	var body struct {
		Error struct {
			Status string `json:"status"`
			Code   any    `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(r, 64<<10)).Decode(&body)
	code := body.Error.Status
	if c, ok := body.Error.Code.(string); ok && code == "" {
		code = c
	}
	if len(code) > 64 || strings.Trim(code, _codeChars) != "" {
		return ""
	}
	return code
}

// Backfill embeds items that have no vector and returns how many it filled. It stops at
// the first embed error.
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

// ClearVectors drops every stored vector, so Backfill re-embeds with the current provider.
func (s *Store) ClearVectors(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE memory SET vector = NULL`); err != nil {
		return fmt.Errorf("clear vectors: %w", err)
	}
	return nil
}

// Nearest returns up to k items that pass f and have a vector with a cosine similarity of
// at least the embedder's MinCosine, closest first. Score is the cosine similarity; ties go to accepted before
// retired, then newer.
func (s *Store) Nearest(ctx context.Context, query string, f Filter, k int) ([]Match, error) {
	if k <= 0 || len(f.Scopes) == 0 {
		return nil, nil
	}
	q, err := s.embed.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", _errEmbedQuery, err)
	}
	if q == nil {
		return nil, nil
	}
	hits, err := s.scoreVectors(ctx, q, f)
	if err != nil {
		return nil, err
	}
	hits = slices.DeleteFunc(hits, func(h Match) bool { return h.Score < s.embed.MinCosine() })
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits, nil
}

func (s *Store) scoreVectors(ctx context.Context, q []float32, f Filter) ([]Match, error) {
	where, args := f.where()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, status, vector FROM memory WHERE vector IS NOT NULL AND `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("query vectors: %w", err)
	}
	defer rows.Close()
	var hits []Match
	for rows.Next() {
		var (
			h    Match
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
		if (hits[i].Status == StatusAccepted) != (hits[j].Status == StatusAccepted) {
			return hits[i].Status == StatusAccepted
		}
		return hits[i].ID > hits[j].ID
	})
	return hits, nil
}

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
