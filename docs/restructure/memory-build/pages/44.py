PAGE = {'n': 44,
 'eyebrow': 'Schema',
 'title': 'memory.Open takes no write lock when up to date',
 'lede': 'Opening the memory tables reads their version without a lock, and refuses a newer '
         'schema.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['83d6944'],
 'what': '`migrate` reads the version once with a plain query. When it is current, no transaction '
         'starts. A version above the last embedded migration fails with `memory tables are newer '
         'than this reef (version N, this reef knows M)`. `apply` re-checks the version inside its '
         'transaction and names the failing step.',
 'why': 'The store opens SQLite with immediate transactions, so every `reef memory show` or '
        '`recall` took the write lock twice and failed with `database is locked` while `reef '
        'serve` wrote. An older binary also ran on newer memory tables with old rules.',
 'flow': 'memory.Open -> migrate                memory/memory.go:514\n'
         '  version (no tx)                      memory/memory.go:558\n'
         '  > known -> error                     :542\n'
         '  == known -> return\n'
         '  else apply each pending (tx, re-check) memory/memory.go:580',
 'changes': [{'h': 'memory/memory.go',
              'text': ['`migration` type, `version` helper, lock-free fast path, newer-schema '
                       'error, step names.'],
              'where': ['memory/memory.go:514']},
             {'h': 'memory/memory_test.go',
              'text': ['`TestOpen_UpToDateNeedsNoWriteLock`, '
                       '`TestOpen_RefusesNewerMemorySchema`.']}],
 'decisions': [],
 'proof': [{'check': 'Open succeeds while another connection holds the write lock',
            'how': 'failed before: `apply 001_items.sql: database is locked (5) (SQLITE_BUSY)`',
            'test': 'TestOpen_UpToDateNeedsNoWriteLock'},
           {'check': 'Newer memory schema refused',
            'how': 'failed before: `Open err = <nil>`',
            'test': 'TestOpen_RefusesNewerMemorySchema'},
           {'check': 'Upgrade and concurrent open',
            'how': 'pass; real round-1 db upgraded by tip binary (blast radius)',
            'test': 'TestOpen_UpgradesTheFirstMemorySchema, store TestOpenConcurrent'}],
 'try': ['Hold `BEGIN IMMEDIATE` on reef.db in another shell, run `reef memory recall x`'],
 'left': []}
