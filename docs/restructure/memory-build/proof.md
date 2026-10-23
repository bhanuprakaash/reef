# Memory service: proof

One section per issue. Each lists the exact commands run on branch `memory-service` and what they printed.
Pages per ticket: open `index.html` in this folder.

## #23 Memory tables and item writes

Commit `63a81b3`.

```
$ go vet ./memory ./store && go test ./memory ./store
ok  	github.com/bhanuprakaash/reef/memory	0.010s
ok  	github.com/bhanuprakaash/reef/store	2.114s

$ go test -count=1 -v ./memory | grep -E "^--- "
--- PASS: TestOpen_Idempotent (0.00s)
--- PASS: TestSchema_NoForeignKeys (0.00s)
--- PASS: TestRemember_WritesItemAndLinks (0.00s)
--- PASS: TestRemember_Invalid (0.00s)
--- PASS: TestRemember_ReplacesRetiresOld (0.00s)
--- PASS: TestRemember_ReplacesRetiredFailsAndWritesNothing (0.00s)
--- PASS: TestRetire_NothingReplacing (0.00s)
--- PASS: TestAccept_CandidateBecomesAccepted (0.00s)
--- PASS: TestRead_Missing (0.00s)

$ go list -deps ./memory | grep reef
github.com/bhanuprakaash/reef/memory

$ grep -ci references memory/schema.sql
0
```

- Open twice: `TestOpen_Idempotent` opens a fresh `:memory:` db, opens again, checks both tables exist and a stored row survives the second open.
- `store.Store` gains only `func (s *Store) DB() *sql.DB` (store/store.go:85).

## #24 Write authority and candidate status

Commit `1643565`.

```
$ go test -count=1 -run TestRemember_Status -v ./memory | grep -E "^\s*--- "
--- PASS: TestRemember_Status (0.01s)
    --- PASS: TestRemember_Status/convention_by_developer (0.00s)
    --- PASS: TestRemember_Status/convention_by_architect (0.00s)
    --- PASS: TestRemember_Status/convention_by_user (0.00s)
    --- PASS: TestRemember_Status/decision_by_developer (0.00s)
    --- PASS: TestRemember_Status/decision_by_tester (0.00s)
    --- PASS: TestRemember_Status/finding_by_tester (0.00s)
    --- PASS: TestRemember_Status/lesson_in_team_scope_by_lead (0.00s)
    --- PASS: TestRemember_Status/lesson_in_project_scope_by_developer (0.00s)
```

Rule (memory/memory.go `statusFor`): author `user` is accepted; team scope from anyone else is candidate; a convention is accepted only from architect or lead; every other kind is accepted from any role.

## #25 Full-text search

Commit `302f1fb`.

```
$ go test -count=1 -run Search -v ./memory | grep -E "^\s*--- "
--- PASS: TestSearch_StemsWords (0.00s)
--- PASS: TestSearch_OnlyAllowedScopes (0.00s)
--- PASS: TestSearch_RetiredAndCandidates (0.00s)
--- PASS: TestSearch_FindsLaterWrites (0.00s)
--- PASS: TestSearch_QueryNeverBreaksSyntax (0.00s)
```

## #26 Embeddings, optional

Commit `97b297b`. The `none` test also runs Search: it was merged after the full-text search ticket.

```
$ go test -count=1 -v -run 'TestNone|TestNearest|TestRemember_StoresWhen|TestEmbedders|TestNewEmbedder' ./memory | grep -E '^\s*--- '
--- PASS: TestNone_MakesNoHTTPCall (0.01s)
    --- PASS: TestNone_MakesNoHTTPCall/empty_config (0.00s)
    --- PASS: TestNone_MakesNoHTTPCall/provider_none_with_key (0.00s)
    --- PASS: TestNone_MakesNoHTTPCall/openai_without_key (0.00s)
    --- PASS: TestNone_MakesNoHTTPCall/voyage_without_key (0.00s)
--- PASS: TestNearest_OrdersByCosine (0.00s)
--- PASS: TestRemember_StoresWhenEmbedderFailsThenBackfills (0.00s)
--- PASS: TestEmbedders_VendorShapes (0.00s)
    --- PASS: TestEmbedders_VendorShapes/openai (0.00s)
    --- PASS: TestEmbedders_VendorShapes/voyage (0.00s)
--- PASS: TestEmbedders_ErrorsNameStatusNotKey (0.00s)
    --- PASS: TestEmbedders_ErrorsNameStatusNotKey/openai (0.00s)
    --- PASS: TestEmbedders_ErrorsNameStatusNotKey/voyage (0.00s)
--- PASS: TestNewEmbedder_UnknownProvider (0.00s)
```

- No HTTP call with none: `TestNone_MakesNoHTTPCall` uses an http.Client whose transport fails the test on any request; Remember, Backfill, Nearest and Search all run.
- Vendor shapes: `TestEmbedders_VendorShapes` checks POST /v1/embeddings, `Authorization: Bearer`, request bodies `{"model":"text-embedding-3-small","input":"hello"}` (OpenAI) and `{"model":"voyage-3-lite","input":["hello"]}` (Voyage), and parses `data[0].embedding`.

## #27 Hybrid recall with link expansion

Commit `5981813`.

```
$ go test -count=1 -run Recall -v ./memory | grep -E "^\s*--- "
--- PASS: TestRecall_NoneEmbedderIsSearchPlusExpansion (0.00s)
--- PASS: TestRecall_ExpansionReachesReplacedItem (0.00s)
--- PASS: TestRecall_ExpansionSkipsHubs (0.01s)
--- PASS: TestRecall_NewestAcceptedConflictFirst (0.00s)
--- PASS: TestRecall_NeverExceedsLimit (0.01s)
    --- PASS: TestRecall_NeverExceedsLimit/set (0.00s)
    --- PASS: TestRecall_NeverExceedsLimit/default (0.00s)
--- PASS: TestRecall_FusesMeaningMatches (0.00s)
--- PASS: TestRecall_EmbedFailureFallsBackToWords (0.00s)
--- PASS: TestRecall_ScopeRestriction (0.00s)
    --- PASS: TestRecall_ScopeRestriction/all_visible (0.00s)
    --- PASS: TestRecall_ScopeRestriction/restricted (0.00s)
    --- PASS: TestRecall_ScopeRestriction/restricted_to_hidden (0.00s)
    --- PASS: TestRecall_ScopeRestriction/none_visible (0.00s)
```

- Hub rule: `TestRecall_ExpansionSkipsHubs` seeds 50 items all linked `about project:1`, each also about its own work item, plus one more about work_item:7. Query `7` with limit 100 returns ids `[7 51]`, via `[direct, about work_item:7]`, not 50.
- Replaced: query `failure` returns `[2 1]`, via `[direct, replaces memory:1]`, id 1 marked retired.

## #28 Lookalike check on write

Commit `76cebf6`.

```
$ go test -count=1 -run Lookalike -v ./memory | grep -E "^\s*--- "
--- PASS: TestLookalike_NearDuplicateWithoutVectors (0.00s)
--- PASS: TestLookalike_UnrelatedWithoutVectors (0.00s)
    --- PASS: TestLookalike_UnrelatedWithoutVectors/no_shared_words (0.00s)
    --- PASS: TestLookalike_UnrelatedWithoutVectors/under_the_term_share (0.00s)
--- PASS: TestLookalike_OtherScopeIgnored (0.00s)
--- PASS: TestLookalike_CosineThreshold (0.00s)
--- PASS: TestLookalike_ReplacedItemNotReported (0.00s)
    --- PASS: TestLookalike_ReplacedItemNotReported/words (0.00s)
    --- PASS: TestLookalike_ReplacedItemNotReported/vectors (0.00s)

$ grep -n "_similar" memory/*.go | grep -v _test
memory/lookalike.go:10:	_similarCosine    = 0.92
memory/lookalike.go:11:	_similarTermShare = 0.7
memory/lookalike.go:33:			if h.Score < _similarCosine {
memory/lookalike.go:68:	if float64(found)/float64(len(want)) < _similarTermShare {
```

- Near-duplicate: "retry the upload three times" then "retry the upload three times on failure" returns similar `[{1 retry the upload three times}]` and the table holds 2 items.

## #29 Eval harness

Commits `37d02ca` (harness) and `f8f1131` (fix the eval found: a retired item outranked a live one on equal scores).

```
$ go test -count=1 ./memory/eval
ok  	github.com/bhanuprakaash/reef/memory/eval	0.053s

$ go test -count=1 -v ./memory/eval | grep -E "^eval:|SKIP|FAIL"
eval: 26 scenarios, 3 skipped, recall@10=1.000 mrr=1.000 (thresholds 1.00, 1.00)
    --- SKIP: TestRecall/synonym-login-expiry (0.00s)
    --- SKIP: TestRecall/synonym-flaky-tests (0.00s)
    --- SKIP: TestRecall/synonym-database-choice (0.00s)

$ go test -count=1 -v ./memory/eval | grep -cE "^    --- PASS"
26

$ go test -count=1 -tags embed ./memory/eval   # no key in env: synonym cases skip
ok  	github.com/bhanuprakaash/reef/memory/eval	0.052s
```

- Prints recall@10 and MRR: the `eval:` line. `go test` hides a passing package's output in package-list mode, so it shows with `-v` or from inside `memory/eval`.
- Forbidden check: adding `burst` (which is returned) to the forbidden list of `direct-two-matches` failed with `forbidden burst returned; results: [limit burst]`; reverted.
- Thresholds after the first green run: recall@10 1.0, MRR 0.98. After the tie fix: MRR 1.0. Recorded in the run file and in `memory/eval/fixture.json`.
- Tie fix, test first: `TestRecall_LiveBeforeNewerRetiredOnEqualScore` failed with `ids = [2 1], want [1 2]` before the fix and passes after.
- Not run: synonym cases against a real embedder (no key here).

## #30 reef memory CLI

Commit `de1fabc`.

```
$ go test -count=1 ./cmd/reef
ok  	github.com/bhanuprakaash/reef/cmd/reef	1.161s

$ go test -count=1 -v -run 'TestCLI_Memory' ./cmd/reef | grep -E '^--- '
--- PASS: TestCLI_Memory (0.02s)
--- PASS: TestCLI_MemoryItems (0.04s)
```

Real binary (`go build -o /tmp/reef-proof ./cmd/reef`) on a fresh data dir, from a fresh folder; every call has `--data $D`:

```
$ reef project add --name shop
1  shop  /tmp/reef-proof-proj.6V6z
$ reef memory add decision "Use sessions, not JWT" --scope project:1 --why stateless
1  accepted  decision  project:1  Use sessions, not JWT
$ reef memory add convention "Wrap errors with %w" --scope team:core --as developer
2  accepted  convention  team:core  Wrap errors with %w
$ reef memory add decision "Use signed cookie sessions" --scope project:1 --about work_item:12 --replaces 1
3  accepted  decision  project:1  Use signed cookie sessions
$ reef memory recall sessions
3  accepted  decision  project:1  direct  Use signed cookie sessions
1  retired  decision  project:1  direct  Use sessions, not JWT
$ reef memory show 3
3  accepted  decision  project:1  Use signed cookie sessions
author  user
link  about  work_item:12
link  replaces  memory:1
$ python3 sqlite: UPDATE memory SET status='candidate' WHERE id=2   # the CLI writes as user, so it never makes a candidate
$ reef memory pending
2  candidate  convention  team:core  Wrap errors with %w
$ reef memory approve 2
2  accepted
$ reef memory pending
$ reef memory approve 3
reef: invalid: memory 3 is not a candidate
exit=1
$ reef memory retire 3
3  retired
$ echo "blob body" | reef memory set -
saved memory for project shop
$ reef memory
blob body
$ reef memory show
blob body
```

- Expanded via: `TestCLI_MemoryItems` checks `recall cookie` prints `1  retired  decision  project:1  replaces memory:1  Use JWT for sessions`.
- docs/memory.md CLI table rows 84 to 89 list the new commands; rows 82 and 83 say the blob commands leave with the context assembler.
