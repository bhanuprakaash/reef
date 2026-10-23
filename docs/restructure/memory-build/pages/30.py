PAGE = {
    "n": 30,
    "eyebrow": "CLI",
    "title": "reef memory CLI",
    "lede": "Six new `reef memory` commands write, find, show, retire and approve memory items from the terminal.",
    "branch": "memory-service",
    "base": "docs/restructure-proposal",
    "commits": ["de1fabc"],
    "what": "`reef memory add` writes an item as author user, so it is always accepted. `recall` searches every scope and prints status and `via` per hit. `show <id>` prints one item with its why, author, links and retirement. `retire`, `pending` and `approve` manage status. Bare `reef memory`, `show` with no id and `set` still handle the old project blob.",
    "why": "The user is the one writer the CLI has (R1, R21). The blob commands stay until the context assembler replaces project memory in `channel/context.go`.",
    "flow": """reef memory recall cookie --data D            cmd/reef/memory.go:193
  store.Open(D) -> memory.Open(st.DB(), nil)
  mem.Scopes(ctx)  every scope with an item     memory/memory.go
  mem.Recall(Query{Scopes, Text, Limit, Scope})
  print: id  status  kind  scope  via  body
reef memory approve 3
  mem.Accept -> ErrInvalid "memory 3 is not a candidate"
  main: "reef: invalid: memory 3 is not a candidate", exit 1""",
    "changes": [
        {"h": "cmd/reef/memory.go", "text": ["Routing for add, recall, show <id>, retire, pending, approve; a repeatable flag type for --about and --replaces; kind:id parsing."], "where": ["cmd/reef/memory.go:43", "cmd/reef/memory.go:153", "cmd/reef/memory.go:193"]},
        {"h": "memory/memory.go", "text": ["`ErrInvalid` wraps bad input and wrong-state errors. `Scopes(ctx)` lists every scope that holds an item."]},
        {"h": "docs/memory.md", "text": ["Six new rows in the CLI table; the two blob rows say they leave with the assembler."], "where": ["docs/memory.md:82"]},
    ],
    "decisions": [
        {"h": "All scopes", "text": ["The CLI's visible scopes are the distinct scopes in the memory table."]},
        {"h": "Scope format", "text": ["`kind:id`, for example `project:1` or `work_item:12`."]},
    ],
    "proof": [
        {"check": "Each new command on a fresh reef.db, id first", "how": "real binary run in proof.md; candidate made with SQL because the CLI writes as user", "test": "TestCLI_MemoryItems"},
        {"check": "Blob commands still print the blob", "how": "`reef memory` and `reef memory show` print `blob body`", "test": "TestCLI_Memory"},
        {"check": "recall shows status and via", "how": "`1  retired  ...  replaces memory:1  ...`", "test": "TestCLI_MemoryItems"},
        {"check": "approve of a non-candidate", "how": "`reef: invalid: memory 3 is not a candidate`, exit 1", "test": "TestCLI_MemoryItems"},
        {"check": "docs/memory.md table", "how": "rows 82 to 89", "test": "review"},
    ],
    "try": ["`reef memory add decision \"Use sessions, not JWT\" --scope project:1`", "`reef memory recall sessions`", "`reef memory show 1`"],
    "left": [
        {"h": "Unknown scope kind in recall", "text": ["`recall --scope projet:1` returns nothing instead of an error."]},
        {"h": "Newlines in a body", "text": ["A body with a newline prints on two lines."]},
    ],
}
