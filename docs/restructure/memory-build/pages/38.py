PAGE = {'n': 38,
 'eyebrow': 'Embeddings',
 'title': 'Visible embedding failures and rate limits',
 'lede': 'A write that could not store a vector says so, and a rate limit gets one retry.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['aa726c3', '219d2f5'],
 'what': '`Result.EmbedErr` holds why no vector was stored (nil when stored or when no embedder is '
         'set). `reef memory add` prints `vector not stored: <reason>; run reef memory embed '
         '<provider> to retry` on stderr and exits 0. Vendor errors name the status and the vendor '
         'error code, such as `status 429 RESOURCE_EXHAUSTED`, never the message. A 429 waits for '
         'Retry-After (1 to 60 s) and retries once.',
 'why': 'Before, a failed embed was silent: the note lost meaning search and nobody knew. R30 '
        'records that the background pass waits for server wiring; until then the warning tells '
        'the user to re-run embed.',
 'flow': 'reef memory add ...                    cmd/reef/memory.go\n'
         '  Remember -> Embed -> postJSON        memory/embed.go:325\n'
         '    429 -> wait retryAfter -> once more  (moved to Backfill by #45)\n'
         '    non-2xx -> "status N <vendorCode>"   :368\n'
         '  Result.EmbedErr                      memory/memory.go:116\n'
         '  stderr: vector not stored ...        cmd/reef/memory.go:216',
 'changes': [{'h': 'memory/memory.go',
              'text': ['`Result.EmbedErr`; Remember keeps the error.'],
              'where': ['memory/memory.go:116']},
             {'h': 'memory/embed.go',
              'text': ['`postJSON` retry, `retryAfter`, `vendorCode` (token of letters, digits, '
                       '`_.-`, at most 64).'],
              'where': ['memory/embed.go:325', 'memory/embed.go:368']},
             {'h': 'cmd/reef',
              'text': ['`memoryCmd` takes stderr; `memoryAdd` prints the warning with the stored '
                       'provider.']},
             {'h': 'docs', 'text': ['R30 in decisions.md; docs/memory.md add row.']}],
 'decisions': [{'h': 'An error, not a bool',
                'text': ['The CLI needs the reason, so `EmbedErr error` instead of `Embedded '
                         'bool`.']},
               {'h': 'Warning names the provider',
                'text': ['Bare `reef memory embed` only prints the provider; the command that '
                         'retries is `reef memory embed openai`.']}],
 'proof': [{'check': 'Failing embedder: stored, EmbedErr set; none: no warning',
            'how': 'passes',
            'test': 'TestRemember_ReportsEmbedFailure'},
           {'check': 'CLI warns on stderr, exit 0, no key',
            'how': '`vector not stored: openai embeddings: status 401 invalid_api_key; run reef '
                   'memory embed openai to retry`',
            'test': 'TestCLI_MemoryAddWarnsWhenVectorNotStored'},
           {'check': '429 then 200: two requests; 429 twice fails',
            'how': 'failed before: `status 429`, `1 requests, want 2`',
            'test': 'TestEmbedders_RetryOnceOn429'},
           {'check': 'Vendor code named, message not',
            'how': 'RESOURCE_EXHAUSTED, insufficient_quota',
            'test': 'TestEmbedders_ErrorNamesVendorCodeNotMessage'},
           {'check': 'R30 recorded', 'how': 'decisions.md', 'test': '-'}],
 'try': ['Set a wrong key, then `reef memory add fact x --scope project:1`'],
 'left': [{'h': 'No background pass', 'text': ['No background pass (R30).']},
          {'h': 'Retry moved by #45',
           'text': ['#45 moved the 429 wait out of `postJSON` into `Backfill`. Writes and recall '
                    'no longer wait; `TestEmbedders_RetryOnceOn429` became '
                    '`TestBackfill_WaitsOutOne429PerItem`.']}]}
