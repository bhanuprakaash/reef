PAGE = {'n': 23,
 'eyebrow': 'Foundation',
 'title': 'Memory tables and item writes',
 'lede': 'Package memory/ gets its own two tables and the calls that write, read, retire and '
         'accept one memory item.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['63a81b3', 'd60217d'],
 'what': 'A memory item is one statement, for example `Use sessions, not JWT`. It has a kind, a '
         'scope, an author and role, and a status. Links point from the item to plain string '
         'targets. `Remember` writes the item and its links in one transaction. A note can name '
         'items it replaces; each replaced item becomes retired and points at the new item.',
 'why': 'Decisions R15 to R18 settle that memory is many small items, append-only, with links that '
        'are plain strings. The package owns its tables, so the store migration runner does not '
        'change. The caller passes the open database through a one-line `DB()` accessor.',
 'flow': 'caller: store.Open(dir) -> st.DB()                       store/store.go:85\n'
         '  memory.Open(db)  applies schema.sql (IF NOT EXISTS)    memory/memory.go:124\n'
         '  s.Remember(ctx, Writer{author, role, scope, links}, Note{kind, body, why, about, '
         'replaces})\n'
         '    validate kind, scope kind, rels                      memory/memory.go:315\n'
         '    BEGIN\n'
         '      INSERT memory (status from statusFor)              memory/memory.go:302\n'
         '      INSERT memory_link per writer link, about, replaces\n'
         '      retire(old, replaced_by = new) per replaces        memory/memory.go:333\n'
         '        old not live -> error, ROLLBACK, nothing written\n'
         '    COMMIT -> Result{Item}',
 'changes': [{'h': 'memory/schema.sql',
              'text': ['Tables `memory` and `memory_link` with indexes. Status has a CHECK; rel '
                       'has a CHECK. No REFERENCES anywhere: targets are strings.'],
              'where': ['memory/schema.sql:1']},
             {'h': 'memory/memory.go',
              'text': ['Types `Ref`, `Link`, `Writer`, `Note`, `Item`, `Result`, `Store`, '
                       'constants for kinds, scopes, rels and statuses.',
                       '`Open`, `Remember`, `Read`, `Retire`, `Accept`, `Pending`. A missing id '
                       'wraps `ErrNotFound`.'],
              'where': ['memory/memory.go:124',
                        'memory/memory.go:138',
                        'memory/memory.go:215',
                        'memory/memory.go:221',
                        'memory/memory.go:273']},
             {'h': 'store/store.go',
              'text': ['One accessor: `DB() *sql.DB`.'],
              'where': ['store/store.go:85']}],
 'decisions': [{'h': 'Replaces link shape',
                'text': ['Stored as rel `replaces`, to_kind `memory`, to_id the old id as text.']},
               {'h': 'Times', 'text': ['RFC 3339 text with nanoseconds.']},
               {'h': 'Empty body', 'text': ['An empty body is an invalid write.']},
               {'h': 'Review fix: candidate replaces',
                'text': ['A candidate that replaces an item keeps its `replaces` link but does not '
                         'retire the old item. `Accept` retires it in one transaction. Before, an '
                         'unapproved write retired an approved item.']}],
 'proof': [{'check': 'Open on a fresh :memory: db, twice',
            'how': 'both tables exist; a row survives the second Open',
            'test': 'TestOpen_Idempotent'},
           {'check': 'Write with links',
            'how': 'writer, about and replaces links read back in order',
            'test': 'TestRemember_WritesItemAndLinks'},
           {'check': 'Replaces retires old',
            'how': 'old status retired, replaced_by = new id',
            'test': 'TestRemember_ReplacesRetiresOld'},
           {'check': 'Replaces of a retired id',
            'how': 'error; item and link counts unchanged',
            'test': 'TestRemember_ReplacesRetiredFailsAndWritesNothing'},
           {'check': 'Retire with nothing replacing',
            'how': 'retired, replaced_by nil',
            'test': 'TestRetire_NothingReplacing'},
           {'check': 'Accept',
            'how': 'candidate becomes accepted; second accept errors',
            'test': 'TestAccept_CandidateBecomesAccepted'},
           {'check': 'No store/channel/server import',
            'how': '`go list -deps ./memory | grep reef` prints only memory',
            'test': 'go list'},
           {'check': 'No foreign keys',
            'how': '`grep -ci references memory/schema.sql` prints 0',
            'test': 'TestSchema_NoForeignKeys'},
           {'check': 'Candidate replace waits for Accept',
            'how': 'old stays accepted until Accept(2), then retired with replaced_by 2',
            'test': 'TestAccept_RetiresWhatTheCandidateReplaces'}],
 'try': ['`go test -v ./memory`'],
 'left': [{'h': 'Write authority',
           'text': ['`statusFor` accepts everything here; the next ticket fills it.']}]}
