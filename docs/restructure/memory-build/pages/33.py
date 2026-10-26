PAGE = {'n': 33,
 'eyebrow': 'Embeddings',
 'title': 'Gemini embedding provider',
 'lede': 'Google Gemini is a third embedding provider. It is free to try, and it has its own '
         'measured cosine floor.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['6ce3a8a', '28903eb', '82edfd6'],
 'what': '`reef memory embed gemini --key K` stores Gemini as the provider. `geminiEmbedder` calls '
         '`gemini-embedding-001` with the key in the `x-goog-api-key` header and asks for 768 '
         'numbers. Each embedder now reports `MinCosine`: the similarity under which a meaning '
         'match is unrelated. Gemini uses 0.6; OpenAI and Voyage keep 0.3. The eval gives the '
         'embedder to every scenario when a key is set, not only the synonym ones.',
 'why': 'R28: Gemini has a free tier, so meaning search costs nothing to try. The real eval showed '
        'that one floor cannot fit every model: with 0.3, Gemini let unrelated notes into 13 of 29 '
        'scenarios.',
 'flow': 'reef memory embed gemini --key K       cmd/reef/memory.go:454\n'
         '  NewEmbedder(gemini)                  memory/embed.go:79\n'
         '  geminiEmbedder.Embed                 memory/embed.go:127\n'
         '    POST .../gemini-embedding-001:embedContent\n'
         '    {"content":{"parts":[{"text":..}]},"outputDimensionality":768}\n'
         '    <- {"embedding":{"values":[768 numbers]}}\n'
         'Recall -> Nearest                      memory/embed.go:233\n'
         '  drop hits under s.embed.MinCosine()  (gemini 0.6, memory/embed.go:153)',
 'changes': [{'h': 'memory/embed.go',
              'text': ['`geminiEmbedder`, `_geminiURL`, `_geminiDims = 768`. `postJSON` holds the '
                       'HTTP code shared by all three vendors. `Embedder` gains `MinCosine`.'],
              'where': ['memory/embed.go:127', 'memory/embed.go:53', 'memory/embed.go:39']},
             {'h': 'memory/eval/eval_test.go',
              'text': ['Every scenario uses the embedder when one is set. Synonym scenarios still '
                       'skip without one.']},
             {'h': 'cmd/reef',
              'text': ['`gemini` accepted by `reef memory embed`; usage and error text name it.']},
             {'h': 'docs',
              'text': ['R28 records the provider, the floor and the measurement. docs/memory.md '
                       'lists gemini.']}],
 'decisions': [{'h': 'No task type',
                'text': ['Measured with the real API: `RETRIEVAL_QUERY`/`RETRIEVAL_DOCUMENT` and '
                         '`SEMANTIC_SIMILARITY` raise every score but do not widen the gap between '
                         'related and unrelated notes. So the request sends none, and the '
                         '`Embedder` interface stays one-way.']},
               {'h': 'Floor 0.6 for Gemini',
                'text': ['Related notes scored 0.685 to 0.758; unrelated at most 0.569. 0.6 sits '
                         'in the gap. OpenAI and Voyage stay 0.3 until someone runs the eval with '
                         'their keys.']},
               {'h': 'Lookalike stays 0.92',
                'text': ['Near-duplicates scored 0.98; reworded notes 0.84 to 0.87; unrelated '
                         '0.52. Only near-copies are flagged.']}],
 'proof': [{'check': 'Request shape, header and parsing',
            'how': 'fake server checks path, `x-goog-api-key`, body with 768, parses values',
            'test': 'TestEmbedders_VendorShapes/gemini'},
           {'check': 'Empty values fail for all three vendors',
            'how': '`gemini embeddings: no embedding in response`; fails when the check is removed',
            'test': 'TestEmbedders_EmptyEmbeddingFails'},
           {'check': 'CLI without key fails, with key embeds, key never printed',
            'how': 'real binary: `invalid: embedding provider gemini needs --key`, then `embedded '
                   '2 items`; key count in output 0',
            'test': 'TestCLI_MemoryEmbed'},
           {'check': 'Real API: meaning match with no shared words',
            'how': 'recall "how long can a user stay signed in while idle" returns "Sessions '
                   'expire after fifteen minutes of inactivity"; coffee note not returned',
            'test': 'real binary'},
           {'check': 'Real eval passes',
            'how': '`eval: 30 scenarios, 0 skipped, recall@10=1.000 mrr=1.000`',
            'test': 'go test -tags embed ./memory/eval'},
           {'check': 'Docs', 'how': 'R28, docs/memory.md, restructure docs R15–R30', 'test': '-'}],
 'try': ['Store a key: see docs/memory.md, then `reef memory embed gemini --key K`',
         '`reef memory recall "how long can a user stay signed in while idle"`'],
 'left': [{'h': 'OpenAI and Voyage floors are still guesses',
           'text': ['OpenAI and Voyage floors are still guesses (no keys).']},
          {'h': 'Free tier caps requests per minute',
           'text': ['Free tier caps requests per minute; a large backfill may need a re-run (see '
                    '#38).']}]}
