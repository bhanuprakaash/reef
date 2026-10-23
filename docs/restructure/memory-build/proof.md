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
