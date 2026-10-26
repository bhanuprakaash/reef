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

## #31 Embedding settings

Commit `4b4ff25`.

```
$ go test -count=1 ./store ./cmd/reef
ok  	github.com/bhanuprakaash/reef/store	2.187s
ok  	github.com/bhanuprakaash/reef/cmd/reef	1.183s

$ go test -count=1 -v -run 'TestCLI_MemoryEmbed|TestSetting' ./cmd/reef ./store | grep -E '^--- '
--- PASS: TestCLI_MemoryEmbed (0.03s)
```

- Vector with a key, none without: `TestCLI_MemoryEmbed` swaps `http.DefaultClient.Transport` for a fake that answers the OpenAI shape. `memory add` before any setting stores 0 vectors; `memory embed openai --key sk-test-SECRET` prints `embedding provider openai; embedded 1 items`; every item then has a vector; the next `memory add` stores one; the fake saw `Bearer sk-test-SECRET`.
- Key never printed: the same test collects stdout and stderr of every command and finds no `sk-test-SECRET`.

Real binary on a fresh data dir, key `sk-test-SECRET-7f3a` (fake; the real OpenAI API answered 429):

```
$ reef memory embed
none
$ reef memory add decision "Use sessions, not JWT" --scope project:1
1  accepted  decision  project:1  Use sessions, not JWT
vectors stored: 0
$ reef memory embed openai --key <KEY>
reef: embed 1: openai embeddings: status 429
exit=1
$ reef memory embed
openai
$ reef memory show 1
1  accepted  decision  project:1  Use sessions, not JWT
author  user
$ reef memory recall sessions
1  accepted  decision  project:1  direct  Use sessions, not JWT
$ reef memory embed bogus
reef: invalid: embedding provider "bogus" is not none, openai or voyage
exit=1
$ reef serve --addr 127.0.0.1:7791 ; curl /projects /providers /profile /features /channels /inbox /decisions /search?q=sessions
(real JSON bodies, 701 bytes with the CLI output)
grep -c key in CLI + REST + server log output: 0
key stored in db: 1
```

- No REST route reads the `setting` table: `grep -rn setting server/*.go` matches only an unrelated GitHub comment and test name.

## Slice verification

Branch tip `19dc312`.

### Full gate

```
$ go vet ./...
(no output)

$ go test -count=1 ./... 2>&1 | grep -E '^(ok|FAIL|---)' | grep -v '^ok'
--- FAIL: TestStartGitHubPoll_PollsOnceAtStart (5.04s)
--- FAIL: TestStartGitHubPoll_ConstructionDoesNotBlockOnSlowGh (10.04s)
--- FAIL: TestPollGitHub_PRAndCheckNotesDedupeAndSurviveRestart (0.04s)
--- FAIL: TestPollGitHub_NoteNotLostBeforeFeatureHasChannel (0.04s)
--- FAIL: TestPollGitHub_NotePublishesToOpenChannel (5.04s)
--- FAIL: TestPollGitHub_NewCheckRunNotesAgain (0.04s)
--- FAIL: TestPollGitHub_TwoRunsSameNameBothNoted (0.05s)
--- FAIL: TestPollGitHub_TimedOutAndStartupFailureCountAsFailed (0.07s)
--- FAIL: TestPollGitHub_FailedCheckWakesDeveloper (5.04s)
--- FAIL: TestPollGitHub_SendNotRefusedDuringLogFetch (5.04s)
--- FAIL: TestPollGitHub_FailedCheckWakeRetriesWhenChannelBusy (5.04s)
--- FAIL: TestPollGitHub_SoloTeamFailedCheckNotesOnlyNoWake (0.04s)
--- FAIL: TestPollGitHub_TwoFailedChecksOneWake (5.04s)
--- FAIL: TestPollGitHub_FailedChecksCountAsHopsAndPauseAtCap (5.05s)
FAIL
FAIL	github.com/bhanuprakaash/reef/server	63.040s
FAIL

$ go test -count=1 ./memory/... ./cmd/reef ./store
ok  	github.com/bhanuprakaash/reef/memory	0.208s
ok  	github.com/bhanuprakaash/reef/memory/eval	0.090s
ok  	github.com/bhanuprakaash/reef/cmd/reef	1.256s
ok  	github.com/bhanuprakaash/reef/store	2.264s

$ go test -count=1 -v ./memory/eval | grep '^eval:'
eval: 27 scenarios, 3 skipped, recall@10=1.000 mrr=1.000 (thresholds 1.00, 1.00)

$ go list -deps ./memory/... | grep reef
github.com/bhanuprakaash/reef/memory
github.com/bhanuprakaash/reef/memory/eval

$ git diff --stat 44bc27b HEAD -- channel server web
(no output)
```

- `server/` failures are not from this branch: the branch changes nothing in server/, channel/ or web/. The same 14 GitHub-poll tests fail on `origin/main` and on the base `44bc27b`. Cause: `store/channel.go` `FeatureLatestChannel` selects 12 columns (no `c.active_turn`) and `scanChannel` scans 13; introduced by `2222c9e`. One-line fix: add `c.active_turn` after `c.closed_at`. Not fixed here: this build may not touch store/ beyond the setting table.
- `TestDirectChainEndClearsActiveTurn` in server/ is flaky on `origin/main` too (failed 2 of 5 runs there, 3 of 5 on the base).
- An opus agent re-ran all 39 acceptance lines of #23–#31 against the tip: 39 PASS. NOT RUN: the synonym eval cases and the vendor clients against a real API (no key). For #31 it put a local TLS proxy in the vendors' response shape in front of the real binary: `embedded 8 items`, every item with a vector; a switch to voyage cleared and refilled them; the key matched 0 times in all CLI output, 20 REST GET routes and the server log.

### Interrogate (opus + sonnet) and comment-sicko

Fixed (each with a test that failed first):
- A candidate that replaced an item retired the accepted original at write time. Now `Accept` does the retire (`d60217d`). Both models.
- `embed` reused a stored key for another vendor, never retried a failed backfill, and kept the old model's vectors. Now: key only for the same provider, vectors cleared on a provider change, backfill on every `embed <provider>` (`f535a4c`). Both models.
- With embeddings on, every item became a direct hit. Now `Nearest` drops cosine under `_minCosine = 0.3` (`b436c60`). The value is a guess until the real-embedder eval runs. Opus.
- Expanded neighbours scored parent/2 and never reached the top 10 when ten items matched. Now `s/(1+s)`, one rank below the parent (`c09ae25`); new eval scenario `replaced-reached-among-many-matches`. Opus.
- Every `reef memory` subcommand accepted every flag. Now one flag set per subcommand (`52c470f`). Opus.
- No HTTP timeout on embed calls; DB errors on the vector side were swallowed (`25059f2`). Both models.
- From the gate: `memory embed` missing from the top-level usage; the embed eval with a key and no provider ran silently without an embedder.
- comment-sicko trimmed narration comments (`9079ac8`).

Skipped, with reasons:
- Scope check on `replaces` (both): R1, scopes are relevance filters, not access control.
- `Setting`/`SetSetting` in store (opus): #31 needs to read the table; the run rule allowed the setting table.
- Lookalike does not call Recall (opus): settled in the run file; the check needs raw cosine and the top word match, which the fused score hides.
- Conflict order only by exact tie (sonnet): "newest accepted of two conflicting findings" holds for equal scores; a recency term would be a new ranking decision.
- Eval corpora are small, recall@10 near-vacuous (sonnet): the new many-matches scenario covers expansion; a shared distractor corpus is a follow-up.
- `show <id>` detail rows are not id first (both): the docs table defines them; they describe one item.
- `--key` on the command line (sonnet): the ticket names `--key K`.
- Provider and key written in two statements (opus): one user, one process at a time.

### Blast radius on store/

The store change is the `DB()` accessor, migration 034 (`setting`) and its two methods. The fact it is safe because of: an existing data folder upgrades with its data intact. Proved with real binaries:

```
main binary:   project add, memory set, feature add      -> user_version 33
branch binary: reef memory                               -> "Use Go."  (blob intact), user_version 34
branch binary: memory add, memory embed                  -> "1  accepted  decision  project:1  Use sessions", "none"
main binary:   reef memory                               -> reef: open store: migrate: database is newer than this reef (version 34, this reef knows 33)
```

- After this branch opens a data folder, `main`'s binary refuses it. Forward-only migrations work this way. Try the branch on a copy of your data folder.
- Migration number 034 collides if `main` adds its own 034 before this merges.
- No store table is named `memory`, `memory_link` or `memory_fts`, so `memory.Open` cannot collide with a store migration.

## Round 2: review fixes, Gemini, and a second review (#33–#45)

Branch tip `4ea380a`. Tickets came from the interrogate pass on `13e5f7b` (#33–#41) and a second interrogate pass on `b3ecb2c` (#42–#45). Pages per ticket: `33.html` to `45.html`.

### Decisions added
- R28 Gemini provider: 768 dimensions, no task type, cosine floor 0.6 per provider (OpenAI and Voyage 0.3, not measured). Lookalike 0.92 holds.
- R29 Architect, lead and user conventions accepted on write; any writer may ask for approval (`--needs-approval`).
- R30 No background pass yet; `Backfill` waits out one 429 per item; writes and recall never wait.

### Gemini calibration (real API, 768 dimensions)
```
no task type            related 0.685-0.758   unrelated max 0.569
RETRIEVAL_QUERY/DOC     related 0.738-0.762   unrelated max 0.609
SEMANTIC_SIMILARITY     related 0.794-0.902   unrelated max 0.776
duplicates (no task)    near-copy 0.980  reworded 0.844-0.870  unrelated 0.524
```
With the old 0.3 floor and the embedder given to every scenario, forbidden items leaked in 13 of 29 scenarios. With 0.6:
```
$ REEF_EMBED_PROVIDER=gemini REEF_EMBED_KEY=… go test -tags embed -count=1 -v ./memory/eval
eval: 30 scenarios, 0 skipped, recall@10=1.000 mrr=1.000 (thresholds 1.00, 1.00)
```
The same run found #41: a retired item ranked above the live item that replaced it.

### Real binary against the real Gemini API
```
$ reef memory recall "how long can a user stay signed in while idle"   # no embedder
(empty)
$ reef memory embed gemini
reef: invalid: embedding provider gemini needs --key
$ reef memory embed gemini --key <KEY>
embedding provider gemini; embedded 2 items
$ ls -l reef.db
-rw------- reef.db
$ reef memory recall "how long can a user stay signed in while idle"
1  accepted  decision  project:1  direct  Sessions expire after fifteen minutes of inactivity
```
The coffee item stays out. Key in all captured output: 0.

### Acceptance lines
Two Opus verification passes, real binary and named tests:
- `b3ecb2c`: 82 lines of #23–#31 and #33–#41: 80 PASS, 0 FAIL, 2 NOT RUN (real OpenAI and Voyage, no keys).
- `4ea380a`: 14 lines of #42–#45: 13 PASS, 0 FAIL; the chmod-failure half of one #43 line NOT RUN (needs root); spot re-runs of #27, #28/#36, #31/#35/#39, #37, #38 PASS.

Lock check, real binary, another process holding `BEGIN IMMEDIATE`:
```
before #44 (d81c188): reef: migrate memory tables: apply 001_items.sql: database is locked (5) (SQLITE_BUSY)   4.8 s
after  #44 (4ea380a): 1  accepted  fact  project:1  direct  lock test item                                       0.007 s
memory_version = 99:  reef: migrate memory tables: memory tables are newer than this reef (version 99, this reef knows 2)
```

### Full gate at 4ea380a
```
$ go vet ./...                                        (clean)
$ go test -count=1 ./memory/... ./cmd/reef ./store    ok ×4
$ go test -count=1 ./...                              server only: 14 GitHub-poll tests (fail on main) + flaky TestDirectChainEndClearsActiveTurn
$ git diff --stat 13e5f7b -- channel server web       (empty)
```

### Interrogate round 2 (opus + sonnet on b3ecb2c)
Fixed, each test first: #42 live item above replaced also via link (opus, reproduced); #43 provider and key in one transaction, chmod before the key (both); #44 Open without the write lock, refuse newer memory schema (sonnet reproduced, opus); #45 no wait on writes and recall, no wasted word search, clamp overflow, Backfill skips rejected (both, measured 7.6 s per write at 5000 items). Docs: R28 lookalike numbers, add row with `--needs-approval`, `rejected` in the status list, issue #38 text.

Skipped, with reasons:
- Eval gives the embedder to every scenario (sonnet): deliberate; it is what found the leaks and #41.
- Gemini key header could follow a redirect (sonnet): Google does not redirect; no client change without a case.
- `memoryEmbedSet` opens memory twice (opus): after #44 the second open is a version read, no lock.
- Long usage strings and round-1 lines over 99 columns: usage strings are output; round-1 lines left as they were.

### Blast radius
Round 2 changes `store/` only in `setting.go` (`SetSettings`). The fact it is safe because of: a reef.db written by the round-1 binary upgrades. Proved with both real binaries:
```
round-1 binary 13e5f7b: add 3 items (1 replaced), a candidate      memory_version absent
tip binary:             recall cookie -> 2 accepted, 1 retired via replaces memory:1; memory_version 2
                        reject 1 (old candidate) -> 1 rejected; recall empty; FTS finds old rows
round-1 binary again:   recall still works; rejected item invisible; writes still work
```
