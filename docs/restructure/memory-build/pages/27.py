PAGE = {'n': 27,
 'eyebrow': 'Recall',
 'title': 'Hybrid recall with link expansion',
 'lede': 'One call finds items by words and by meaning, adds their neighbours one link away, ranks '
         'them and cuts at a limit.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['5981813', 'c09ae25'],
 'what': '`Recall(ctx, Query{Scopes, Text, Limit, Scope})` runs Search and Nearest over the '
         "asker's shelves. It fuses the two ranked lists by reciprocal rank fusion: each list adds "
         "1/(60 + rank) to an item's score. From each of the best hits it follows links one step: "
         'links to other items, links that point at the hit, and other items about the same '
         'target. Targets of kind project or team are hubs and are never followed. Each hit says '
         'how it was reached in `Via`, for example `direct` or `replaces memory:1`.',
 'why': "Decision R19: word search and meaning search cover each other's gaps, and one link step "
        'brings the neighbourhood without the whole project. R18: recall may return retired items, '
        'marked retired, so the history of what was believed stays readable.',
 'flow': 'Recall(ctx, Query{Scopes, Text: "failure", Limit: 10})   memory/recall.go:48\n'
         '  Scope set and not in Scopes -> nil\n'
         '  Search(text, Filter{scopes, IncludeRetired}) -> words     memory/search.go\n'
         '  Nearest(text, filter, 50) -> meaning (embed error -> words only; DB error returned) '
         'memory/embed.go\n'
         '  RRF: score += 1/(60+rank) per list, via "direct"\n'
         '  top `limit` direct hits -> neighbours(id)                 memory/recall.go:114\n'
         '     out: links to memory | in: links at the hit | shared non-hub target\n'
         '     score = s/(1+s), one rank below parent, via "<rel> <kind>:<id>"\n'
         '  load(ids) WHERE filter (drops hidden, candidates)         memory/recall.go:144\n'
         '  sort: score, accepted over retired, newer first; cut at limit   memory/recall.go:175',
 'changes': [{'h': 'memory/recall.go',
              'text': ['`Query`, `Hit`, `Recall`, `neighbours`, `load`, `before`.'],
              'where': ['memory/recall.go:48', 'memory/recall.go:114', 'memory/recall.go:144']},
             {'h': 'memory/filter.go, search.go, embed.go',
              'text': ['The search result type is now `Match`, so `Hit` is the recall result the '
                       'ticket names. Search and Nearest break score ties accepted first, then '
                       'newer first.']},
             {'h': 'memory/recall_test.go',
              'text': ['Eight tests: none equals FTS plus expansion, replaced item, hub rule, '
                       'conflict order, limit, meaning fused in, embed failure, scope '
                       'restriction.']}],
 'decisions': [{'h': 'Rename to Match',
                'text': ['The ticket names the recall result `Hit`, so the search result became '
                         '`Match`.']},
               {'h': 'Newer first on ties',
                'text': ['RRF ranks are always distinct, so the lists themselves order equal '
                         'scores accepted first, then newer first. That makes the newest accepted '
                         'of two conflicting findings rank first.']},
               {'h': 'Embed failure',
                'text': ['If Nearest fails, recall answers from words only. Embeddings are '
                         'optional (R20).']},
               {'h': 'Higher score wins',
                'text': ['An item reached twice keeps its higher score. A weak direct hit can then '
                         'show the link it was also reached through.']},
               {'h': 'Review fix: neighbour score',
                'text': ['A neighbour scores `s/(1+s)`: the RRF score one rank below its parent. '
                         "Half the parent's score never beat direct rank 50, so expansion vanished "
                         'once ten items matched.']}],
 'proof': [{'check': 'none embedder: recall = FTS plus expansion',
            'how': 'ids equal Search ids plus expected neighbours',
            'test': 'TestRecall_NoneEmbedderIsSearchPlusExpansion'},
           {'check': 'Reaches the replaced item, retired, via replaces',
            'how': '`[2 1]`, `replaces memory:1`, status retired',
            'test': 'TestRecall_ExpansionReachesReplacedItem'},
           {'check': 'Hub rule with 50 items',
            'how': '`[7 51]`, never 50',
            'test': 'TestRecall_ExpansionSkipsHubs'},
           {'check': 'Newest accepted conflict first',
            'how': '`[2 1]`',
            'test': 'TestRecall_NewestAcceptedConflictFirst'},
           {'check': 'Size never exceeds limit',
            'how': 'set limit and default 10',
            'test': 'TestRecall_NeverExceedsLimit'},
           {'check': 'Expansion survives many direct matches',
            'how': 'replaced item in the top 10 among 12+ matches',
            'test': 'TestRecall_ExpansionSurvivesManyDirectMatches'}],
 'try': ['`go test -run Recall -v ./memory`'],
 'left': [{'h': 'Reranker', 'text': ['None until evals show the need (R19).']}]}
