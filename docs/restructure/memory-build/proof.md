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
