PAGE = {
    "n": 25,
    "eyebrow": "Search",
    "title": "Full-text search",
    "lede": "An FTS5 index over body and why, kept in step by triggers, with a scope and status filter.",
    "branch": "memory-service",
    "base": "docs/restructure-proposal",
    "commits": ["302f1fb"],
    "what": "`Search(query, filter)` returns ids ranked by BM25, best first. The porter tokenizer stems words, so `retries` finds `retry`. The filter names the allowed scopes. Only accepted items come back, plus retired ones marked retired when the filter asks. Candidates never come back.",
    "why": "Decision R19: word-exact search is half of hybrid recall. Scopes are the shelves an asker can see (R1, R16), so an empty scope list returns nothing, never everything.",
    "flow": """INSERT/UPDATE/DELETE memory -> triggers keep memory_fts in step   memory/schema.sql:28
Search(ctx, "retries?", Filter{Scopes, IncludeRetired})              memory/search.go:27
  split into letter/digit terms -> "retries"  (quoted, OR-joined)
  SELECT -bm25 FROM memory_fts JOIN memory
    WHERE MATCH, status IN (accepted[, retired]), scope IN Scopes
  -> []Hit{ID, Score, Status}""",
    "changes": [
        {"h": "memory/schema.sql", "text": ["`memory_fts` external-content table with `porter unicode61`; insert, delete and update-of-text triggers."], "where": ["memory/schema.sql:28"]},
        {"h": "memory/search.go", "text": ["`Filter`, `Hit`, `Search`. The user's text becomes quoted terms, so FTS syntax in the query never errors."], "where": ["memory/search.go:12", "memory/search.go:27"]},
        {"h": "memory/search_test.go", "text": ["Five tests: stemming, scopes, retired and candidates, triggers, query syntax."]},
    ],
    "decisions": [
        {"h": "Score", "text": ["`Score = -bm25()`, so higher is better, the same as cosine later."]},
        {"h": "Empty scope id", "text": ["A scope with an empty id matches only items with an empty id. It is not a wildcard."]},
    ],
    "proof": [
        {"check": "\"retries\" finds \"retry\"", "how": "only id 1 returned, positive score", "test": "TestSearch_StemsWords"},
        {"check": "Outside allowed scopes never returned", "how": "one shelf, two shelves, empty id, no shelves", "test": "TestSearch_OnlyAllowedScopes"},
        {"check": "Retired excluded by default, marked when asked", "how": "status retired on the hit", "test": "TestSearch_RetiredAndCandidates"},
        {"check": "Writes searchable with no extra call", "how": "insert, update, delete after Open", "test": "TestSearch_FindsLaterWrites"},
    ],
    "try": ["`go test -run Search -v ./memory`"],
    "left": [{"h": "No limit in Search", "text": ["Recall cuts at its limit; Search returns every match."]}],
}
