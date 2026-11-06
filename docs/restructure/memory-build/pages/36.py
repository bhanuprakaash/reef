PAGE = {'n': 36,
 'eyebrow': 'Lookalike check',
 'title': 'Lookalike check covers items without vectors',
 'lede': 'A near-duplicate is reported even when the older note was written without an embedder.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['2dfe89c'],
 'what': 'When the new note has a vector, `lookalikes` still compares by cosine, and now also runs '
         'the word check against the first word-search hit that has no vector.',
 'why': 'Before, a vector meant cosine only. Notes stored while the embedder was off or failing '
        'were never compared, so a copy of them passed silently.',
 'flow': 'Remember -> lookalikes(w, n, vec)     memory/lookalike.go:20\n'
         '  vec != nil: cosine >= 0.92 over notes with vectors\n'
         '  Search(n.Body) top hits; skip notes with a vector when vec != nil   :67\n'
         '  first eligible hit: >= 70% shared words -> Similar',
 'changes': [{'h': 'memory/lookalike.go',
              'text': ['No early return after the cosine path. `body` also returns whether the '
                       'note has a vector.'],
              'where': ['memory/lookalike.go:63']},
             {'h': 'memory/lookalike_test.go',
              'text': ['Two tests: found by words, and a replaced note without a vector is not '
                       'reported.']}],
 'decisions': [{'h': 'Word check only for notes without vectors',
                'text': ['A note the embedder already scored below 0.92 is not reported again by '
                         'words. The two paths cover different notes, so no id appears twice.']}],
 'proof': [{'check': 'A stored while the embedder fails; B with embedder reports A',
            'how': 'failed before: `Similar = [], want [{1 retry the upload three times}]`',
            'test': 'TestLookalike_ItemWithoutVectorFoundByWords'},
           {'check': 'Unrelated reports nothing; replaced never reported',
            'how': 'passes',
            'test': 'TestLookalike_ItemWithoutVectorFoundByWords, '
                    'TestLookalike_ReplacedItemWithoutVectorNotReported'},
           {'check': 'Existing lookalike tests',
            'how': 'all pass',
            'test': 'go test -run Lookalike ./memory'}],
 'try': ['Add a note with provider none, switch to a provider, add a near-copy: `N  similar  ...` '
         'is printed'],
 'left': []}
