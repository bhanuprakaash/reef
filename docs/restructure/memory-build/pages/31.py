PAGE = {
    "n": 31,
    "eyebrow": "Settings",
    "title": "Embedding settings",
    "lede": "A `setting` table holds the embedding provider and key. One command turns meaning search on and fills the missing vectors.",
    "branch": "memory-service",
    "base": "docs/restructure-proposal",
    "commits": ["4b4ff25"],
    "what": "Store migration 34 adds `setting` (key, value). `reef memory embed openai --key K` saves `embedding.provider` and `embedding.key`. When embedding was off before, it runs `Backfill` once and prints how many items it embedded. `reef memory embed` prints the provider only. Every `reef memory` command builds its embedder from these two settings.",
    "why": "Decision R20: one field turns on better search, and Reef stays free and local by default. No secrets table existed; MCP keys sit inside team role JSON.",
    "flow": """reef memory embed openai --key K                         cmd/reef/memory.go
  check provider; reuse stored key when --key is empty
  prev := Setting(embedding.provider)                     store/setting.go
  SetSetting(provider), SetSetting(key)
  prev was none or unset -> openMemory -> Backfill -> "embedded N items"
reef memory add ...
  openMemory: Setting x2 -> memory.NewEmbedder -> memory.Open(db, e)""",
    "changes": [
        {"h": "store/migrations/034_setting.sql, store/setting.go", "text": ["The table, `Setting` (missing gives empty) and `SetSetting` (upsert)."], "where": ["store/setting.go:1"]},
        {"h": "cmd/reef/memory.go", "text": ["`openMemory` builds the embedder from settings; the `embed` subcommand and `--key`."]},
        {"h": "store tests", "text": ["user_version 34 in three tests; the skills migration test also drops `setting` when it rewinds."]},
        {"h": "docs/memory.md", "text": ["Two CLI rows for `reef memory embed`."]},
    ],
    "decisions": [
        {"h": "Test fake", "text": ["The CLI test swaps `http.DefaultClient.Transport`, so no production hook exists only for tests."]},
        {"h": "Backfill error", "text": ["The settings are saved first; a backfill error exits 1 with the vendor status, never the key."]},
    ],
    "proof": [
        {"check": "Key set: add stores a vector; none: no vector", "how": "0 vectors before, 1 after add with openai", "test": "TestCLI_MemoryEmbed"},
        {"check": "Switching on runs backfill; every item has a vector", "how": "`embedded 1 items`, count of NULL vectors 0", "test": "TestCLI_MemoryEmbed"},
        {"check": "Key never in CLI or REST output", "how": "test captures all output; real binary + reef serve + 8 GET routes: grep count 0", "test": "TestCLI_MemoryEmbed, hand run"},
    ],
    "try": ["`reef memory embed`", "`reef memory embed openai --key $OPENAI_API_KEY`"],
    "left": [
        {"h": "Settings page and REST route", "text": ["They come with the context assembler."]},
        {"h": "Background fill", "text": ["R20 names a background pass for missing vectors. Not built: Backfill runs only when embedding is switched on."]},
    ],
}
