PAGE = {'n': 42,
 'eyebrow': 'Recall',
 'title': 'Live item always ranks above the item it replaced',
 'lede': 'Closes the hole review found in #41: a live note now ranks above the note it replaced in '
         'every case.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['ab55e97'],
 'what': '`rankBelowReplacers` demotes an item whenever its replacer is in the results, whether '
         'the replacer matched the query or was reached by a link. `load` returns an '
         'id-to-replacer map, so the exported `Hit` type no longer carries a hidden field.',
 'why': 'With v1 → v2 → v3 and two unrelated notes ranked between v2 and v3, link expansion '
        "overwrote v3's `direct` mark, and the #41 rule skipped it: retired v2 came first.",
 'flow': 'Recall                                memory/recall.go\n'
         '  fuse, expand links\n'
         '  load -> hits, replacedBy             memory/recall.go:181\n'
         '  rankBelowReplacers(hits, replacedBy) memory/recall.go:119\n'
         '    replacer in hits -> item score = r/(1+r), replacer first along chains',
 'changes': [{'h': 'memory/recall.go',
              'text': ['No `Via` check; replacer map from `load`.'],
              'where': ['memory/recall.go:119']},
             {'h': 'memory/recall_test.go',
              'text': ['New chain test with unrelated items between; '
                       '`TestRecall_ExpansionReachesReplacedItem` now expects the replacer '
                       'first.']}],
 'decisions': [{'h': 'Live first, always',
                'text': ['Even when only the old wording matched, the agent sees the current note '
                         'first and the old one right under it, marked retired.']}],
 'proof': [{'check': 'Chain with unrelated items between: v3, v2, v1',
            'how': 'failed before: `ids = [2 4 3 1 5], want 3 before 2 before 1`',
            'test': 'TestRecall_LiveBeforeItemItReplacedWhenLinkReachesAReplacer'},
           {'check': 'Old hit directly, replacer by link: replacer first',
            'how': 'failed before: `ids = [1 2], want [2 1]`',
            'test': 'TestRecall_ExpansionReachesReplacedItem'},
           {'check': 'Recall tests, offline eval, Gemini eval',
            'how': 'all pass; Gemini eval 30 scenarios, 1.000 / 1.000',
            'test': 'go test ./memory/...; -tags embed'}],
 'try': ['`go test -run "ExpansionReaches|LiveBefore" -v ./memory`'],
 'left': []}
