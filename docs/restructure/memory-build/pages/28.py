PAGE = {'n': 28,
 'eyebrow': 'Write policy',
 'title': 'Lookalike check on write',
 'lede': 'Remember tells the writer which stored items look like the new one. It never links them '
         'and never refuses the write.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['76cebf6'],
 'what': 'Before the insert, `Remember` embeds the text once. With a vector, every accepted item '
         'in the same scope with cosine at or above 0.92 is similar. Without a vector, the top '
         "word match is similar when it holds at least 70% of the new body's distinct words. The "
         'list comes back in `Result.Similar` as id and body. The same vector is stored with the '
         'item, so nothing is embedded twice.',
 'why': 'Decision R17: links come from the writer, and a lookalike check proposes but never links '
        'silently. The writer then decides to replace the old item or keep both.',
 'flow': 'Remember(ctx, w, n)                                memory/memory.go:138\n'
         '  validate\n'
         '  vec := embed(body + why)   (error -> nil, write goes on)\n'
         '  lookalikes(ctx, w, n, vec)                       memory/lookalike.go:20\n'
         '    vec != nil: scoreVectors in w.Scope, keep cosine >= _similarCosine\n'
         '    vec == nil: Search(body) top match, share of words >= _similarTermShare\n'
         '    drop ids in n.Replaces\n'
         '  BEGIN; INSERT ... vector; links; replaces; COMMIT\n'
         '  -> Result{Item, Similar}',
 'changes': [{'h': 'memory/lookalike.go',
              'text': ['The two thresholds in one const block, `Similar`, `lookalikes`.'],
              'where': ['memory/lookalike.go:10', 'memory/lookalike.go:20']},
             {'h': 'memory/memory.go',
              'text': ['`Result.Similar`. Embed before the transaction and store the vector in the '
                       'INSERT.'],
              'where': ['memory/memory.go:138']},
             {'h': 'memory/embed.go, search.go',
              'text': ['`scoreVectors` shared by Nearest and the check; `terms` shared by Search '
                       'and the check.']}],
 'decisions': [{'h': 'Direct calls, not Recall',
                'text': ["The check needs the raw cosine and the top word match. Recall's fused "
                         'score has neither, so the check calls the vector scorer and Search '
                         'directly.']},
               {'h': 'Replaced ids',
                'text': ['An id the note replaces is never reported; the writer already knows '
                         'it.']}],
 'proof': [{'check': 'Near-duplicate returns earlier id and still stores',
            'how': 'similar `[{1 ...}]`, new id 2, 2 rows',
            'test': 'TestLookalike_NearDuplicateWithoutVectors'},
           {'check': 'Unrelated text returns empty similar',
            'how': 'no shared words; under the share',
            'test': 'TestLookalike_UnrelatedWithoutVectors'},
           {'check': 'Cosine threshold',
            'how': 'fake vectors above and below 0.92',
            'test': 'TestLookalike_CosineThreshold'},
           {'check': 'Two named constants in one place',
            'how': '`grep _similar` shows memory/lookalike.go:10-11 only',
            'test': 'grep'}],
 'try': ['`go test -run Lookalike -v ./memory`'],
 'left': [{'h': 'Stemming in the share',
           'text': ['The word share compares exact lowercase words, not stems. A match found only '
                    'through stemming can fall under 70%.']}]}
