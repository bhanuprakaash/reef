PAGE = {
    "n": 29,
    "eyebrow": "Evals",
    "title": "Eval harness",
    "lede": "Thirty scenarios in one JSON file score recall offline, and any forbidden item in a result fails the test.",
    "branch": "memory-service",
    "base": "docs/restructure-proposal",
    "commits": ["37d02ca", "f8f1131"],
    "what": "`memory/eval/fixture.json` holds the scenarios. Each one seeds items with symbolic names, asks one query with the asker's scopes, and names the expected and the forbidden items. `TestRecall` runs every scenario on a fresh in-memory database. It computes recall@10 (the share of expected items in the top 10) and MRR (one over the rank of the first expected item). It fails under the thresholds in the file. Synonym scenarios need meaning search, so they run only with `-tags embed` and a key in the env.",
    "why": "Decision R26: deterministic checks cover write policy and scope isolation, which published benchmarks skip. Knowledge-update cases (replace, retire, conflict) are in the set.",
    "flow": """go test -v ./memory/eval
  decode fixture.json (unknown fields rejected)
  per scenario: sql.Open(":memory:") -> memory.Open(db, embedder or nil)
    seed items in order (ref -> id), retire the ones marked
    Recall(Query{scopes, text, scope, limit})
    forbidden id in results -> FAIL now
    ordered and out of order -> FAIL now
    recall@10, MRR
  mean over scenarios that ran -> eval: line -> FAIL under thresholds""",
    "changes": [
        {"h": "memory/eval/fixture.json", "text": ["30 scenarios: direct 4, synonym 3, replaced 3, retired 3, conflict 3, cross-team 4, cross-work-item 2, hub rule 2, scope restriction 3, candidates 2. Thresholds recall@10 1.0, MRR 1.0."], "where": ["memory/eval/fixture.json:1"]},
        {"h": "memory/eval/eval_test.go", "text": ["The one test that reads the fixture; no code per scenario."]},
        {"h": "memory/eval/eval_embed_test.go, eval_none_test.go", "text": ["Build-tagged pair: with `embed`, the embedder comes from `REEF_EMBED_PROVIDER` and `REEF_EMBED_KEY`."]},
        {"h": "memory/search.go, embed.go (fix)", "text": ["The eval found a bug: on equal scores the lists put a newer retired item above a live one, and fusion kept that. Both lists now break ties accepted first, then newer first."]},
    ],
    "decisions": [
        {"h": "JSON fixture", "text": ["Standard library only; YAML would be a new dependency."]},
        {"h": "Printing", "text": ["`go test` hides a passing package's output unless `-v` is given, so the numbers show with `-v`."]},
    ],
    "proof": [
        {"check": "Runs offline and prints recall@10 and MRR", "how": "`eval: 26 scenarios, 3 skipped, recall@10=1.000 mrr=1.000` with -v", "test": "TestRecall"},
        {"check": "A forbidden id fails the test", "how": "added a returned id to forbidden: `forbidden burst returned`, FAIL; reverted", "test": "TestRecall/direct-two-matches"},
        {"check": "One readable file, no code per scenario", "how": "fixture.json plus one test function", "test": "review"},
        {"check": "Thresholds recorded after first green run", "how": "run file note: 1.0 / 0.98, then 1.0 / 1.0 after the fix", "test": "run file"},
        {"check": "Tie fix, test first", "how": "failed `ids = [2 1], want [1 2]`, then passed", "test": "TestRecall_LiveBeforeNewerRetiredOnEqualScore"},
        {"check": "Synonym cases with a real embedder", "how": "no key in this environment", "test": "not run"},
    ],
    "try": ["`go test -v ./memory/eval | grep eval:`", "`REEF_EMBED_PROVIDER=openai REEF_EMBED_KEY=... go test -tags embed -v ./memory/eval`"],
    "left": [{"h": "Token cost per wake", "text": ["R26 also names it; it belongs with the context assembler, which is not built yet."]}],
}
