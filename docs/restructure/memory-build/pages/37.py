PAGE = {'n': 37,
 'eyebrow': 'Approval',
 'title': 'Reject a candidate',
 'lede': 'A candidate the user turns down becomes rejected and is never found again.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['0afe99f'],
 'what': 'New status `rejected`. `Store.Reject` works only on candidates; `reef memory reject '
         '<id>` calls it. `Retire` now works only on accepted notes. The memory tables get their '
         'own version table, `memory_version`, and ordered files under `memory/migrations/`. '
         '`002_rejected.sql` rebuilds the `memory` table so its CHECK allows `rejected`.',
 'why': 'Before, rejecting meant retiring, and recall includes retired notes. So a rule the user '
        'refused showed up in recall as history. R18 keeps retired notes as what was believed; a '
        'rejected note never was.',
 'flow': 'reef memory reject 5                  cmd/reef/memory.go:333\n'
         '  Store.Reject                         memory/memory.go:223\n'
         '    UPDATE status=rejected WHERE status=candidate\n'
         'Search/Nearest/Recall/lookalike: Filter matches accepted (+retired) only\n'
         'memory.Open -> migrate                 memory/memory.go:508\n'
         '  apply each migrations/NNN_*.sql above memory_version   :536',
 'changes': [{'h': 'memory/memory.go',
              'text': ['`StatusRejected`, `Reject`, `migrate`/`apply`. `retire` and `checkLive` '
                       'refuse candidates and rejected notes with their own messages.'],
              'where': ['memory/memory.go:223', 'memory/memory.go:360', 'memory/memory.go:508']},
             {'h': 'memory/migrations/',
              'text': ['`001_items.sql` is the old schema.sql unchanged; `002_rejected.sql` copies '
                       'every row and id into a table that allows rejected, and recreates the '
                       'index and FTS triggers.']},
             {'h': 'cmd/reef', 'text': ['`reef memory reject <id>` with its own flags and usage.']},
             {'h': 'docs/memory.md',
              'text': ['reject row; retire row says it refuses candidates.']}],
 'decisions': [{'h': 'Own version table, not user_version',
                'text': ['store/ owns PRAGMA user_version. A db that has `memory` but no '
                         '`memory_version` is at version 1.']},
               {'h': 'A candidate cannot be replaced',
                'text': ['Replace and retire take only accepted notes; a candidate is approved or '
                         'rejected first.']}],
 'proof': [{'check': 'Rejected candidate not recalled; show prints rejected',
            'how': 'real binary: `1  rejected`, recall empty, show `1  rejected  fact ...`',
            'test': 'TestReject_CandidateIsNeverFound'},
           {'check': 'Reject non-candidate / retire candidate fail',
            'how': '`invalid: memory 1 is not a candidate`, `invalid: memory 4 is a candidate`',
            'test': 'TestReject_RefusesWhatIsNotACandidate'},
           {'check': 'Old-schema db upgrades',
            'how': 'unit test, and real round-1 binary db opened by tip: rows, links, FTS, reject '
                   'work; version 2',
            'test': 'TestOpen_UpgradesTheFirstMemorySchema'},
           {'check': 'Rejecting a candidate that replaces leaves the item live',
            'how': 'passes',
            'test': 'TestReject_CandidateReplacingAnItemLeavesItLive'},
           {'check': 'Docs', 'how': 'docs/memory.md reject and retire rows', 'test': '-'}],
 'try': ['`reef memory add convention "X" --scope project:1 --needs-approval`',
         '`reef memory reject <id>`, then `reef memory recall X`'],
 'left': [{'h': '`show` prints `retired_at` for a rejected note',
           'text': ['`show` prints `retired_at` for a rejected note (the reject time).']}]}
