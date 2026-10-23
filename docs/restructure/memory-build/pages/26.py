PAGE = {
    "n": 26,
    "eyebrow": "Search",
    "title": "Embeddings, optional",
    "lede": "Items can carry a vector from OpenAI or Voyage. With no key, nothing calls the network and search stays word-based.",
    "branch": "memory-service",
    "base": "docs/restructure-proposal",
    "commits": ["97b297b"],
    "what": "`NewEmbedder(EmbedConfig{Provider, Key}, client)` gives an `Embedder`. With no key it gives the none embedder, which returns no vector. `Remember` embeds after the item is stored; an embed error never fails the write. `Backfill` fills every missing vector later. `Nearest` ranks items by cosine similarity in Go, over the same scope and status filter that Search uses.",
    "why": "Decision R20: Reef stays free and local by default; one setting turns on better search. R19: vectors are a blob column and cosine runs in Go, so no SQLite extension is needed.",
    "flow": """NewEmbedder({openai, key}, hc)                 memory/embed.go:69
memory.Open(db, emb)                            memory/memory.go
Remember -> COMMIT item -> embedItem (error ignored, vector stays NULL)
Backfill(ctx): SELECT ... WHERE vector IS NULL -> embedItem each   memory/embed.go:128
Nearest(ctx, q, Filter, k)                     memory/embed.go:168
  embed q -> nil? return nil
  SELECT id, status, vector WHERE vector NOT NULL AND Filter.where()   memory/filter.go:22
  cosine in Go -> sort -> top k""",
    "changes": [
        {"h": "memory/embed.go", "text": ["`Embedder`, `EmbedConfig`, OpenAI and Voyage clients sharing one POST helper, `Backfill`, `Nearest`, vector encode and cosine."], "where": ["memory/embed.go:29", "memory/embed.go:69", "memory/embed.go:128", "memory/embed.go:168"]},
        {"h": "memory/filter.go", "text": ["`Filter` and its SQL `where()`, shared by Search and Nearest."], "where": ["memory/filter.go:22"]},
        {"h": "memory/memory.go, schema.sql", "text": ["`Open(db, e)`; a nil embedder means none. Column `vector BLOB`."], "where": ["memory/schema.sql:14"]},
    ],
    "decisions": [
        {"h": "Unknown provider", "text": ["`NewEmbedder` returns an error. A typo must not silently turn recall into words only."]},
        {"h": "Models", "text": ["OpenAI `text-embedding-3-small`, Voyage `voyage-3-lite`."]},
        {"h": "Mixed lengths", "text": ["`Nearest` skips stored vectors of another length, so a provider switch does not break it."]},
    ],
    "proof": [
        {"check": "none: Remember and Search work, no HTTP call", "how": "failing transport; Search finds id 1; 0 vectors stored", "test": "TestNone_MakesNoHTTPCall"},
        {"check": "Nearest ordered by cosine", "how": "fake embedder, literal id order", "test": "TestNearest_OrdersByCosine"},
        {"check": "Write while embedder errors, Backfill fills", "how": "item stored with NULL vector, then filled", "test": "TestRemember_StoresWhenEmbedderFailsThenBackfills"},
        {"check": "Vendor request and response shapes", "how": "httptest.Server checks method, path, auth, JSON", "test": "TestEmbedders_VendorShapes"},
    ],
    "try": ["`go test -run 'TestNone|TestNearest|TestEmbedders' -v ./memory`"],
    "left": [{"h": "Real vendor call", "text": ["Not run here: no keys. The eval ticket runs synonym cases against a real embedder when a key is in the env."]}],
}
