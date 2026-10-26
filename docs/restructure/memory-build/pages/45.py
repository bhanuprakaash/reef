PAGE = {'n': 45,
 'eyebrow': 'Embeddings',
 'title': 'Embedding cost off the hot paths',
 'lede': 'Writes and recall never wait for a rate limit, and the duplicate check skips word search '
         'when every note has a vector.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['d216a4d', '5438a42'],
 'what': '`postJSON` returns a typed `statusError` and never waits. Only `Backfill` waits out one '
         '429 per item (Retry-After clamped to 1 to 60 s before multiplying). `Backfill` skips '
         'rejected notes. `lookalikes` runs its word check only when the scope has a note without '
         'a vector.',
 'why': 'Review measured 7.6 s per write at 5,000 vectored notes, all spent in a word search whose '
        'result was then thrown away. A 429 on the recall query stalled recall for the whole '
        'Retry-After. A huge Retry-After overflowed to a negative wait.',
 'flow': 'Remember/Recall -> Embed -> postJSON  memory/embed.go:325\n'
         '  non-2xx -> statusError (no wait)       memory/embed.go:310\n'
         'Backfill                               memory/embed.go:158\n'
         '  429 -> wait retryAfter (1..60 s), once   :189\n'
         'lookalikes: vec set and no vectorless note in scope -> no word search  '
         'memory/lookalike.go:44',
 'changes': [{'h': 'memory/embed.go',
              'text': ['`statusError`, wait in `Backfill`, clamp in `retryAfter`, rejected '
                       'skipped.'],
              'where': ['memory/embed.go:158', 'memory/embed.go:310']},
             {'h': 'memory/lookalike.go',
              'text': ['one `EXISTS` query decides whether the word path runs.'],
              'where': ['memory/lookalike.go:44']},
             {'h': 'docs', 'text': ["R30 says the wait is Backfill's."]}],
 'decisions': [{'h': 'Prove "no search" without a hook',
                'text': ['The test drops the FTS table: any word search would error, so a clean '
                         'Remember proves none ran. A control test with a vectorless note fails '
                         'the same way.']}],
 'proof': [{'check': 'All vectored: no word search',
            'how': 'failed before: `no such table: memory_fts`',
            'test': 'TestLookalike_AllVectoredRunsNoWordSearch'},
           {'check': 'Recall does not wait on a 429',
            'how': 'failed before: `Recall took 3.0013s`',
            'test': 'TestRecall_NeverWaitsOut429'},
           {'check': 'Backfill 429 then 200 fills; clamp',
            'how': '`retryAfter("9999999999")` was `-2346317h47m`, now 60 s',
            'test': 'TestBackfill_WaitsOutOne429PerItem, TestRetryAfter'},
           {'check': 'Backfill skips rejected',
            'how': 'failed before: `Backfill = 1, want 0`',
            'test': 'TestBackfill_SkipsRejectedItems'},
           {'check': 'R30', 'how': 'decisions.md', 'test': '-'}],
 'try': ['`go test -run "NeverWaits|Backfill_|AllVectored" -v ./memory`'],
 'left': [{'h': 'Word search still unbounded',
           'text': ['When a vectorless note is in scope the OR search still scans every word '
                    'match; fine at personal scale.']}]}
