PAGE = {'n': 41,
 'eyebrow': 'Recall',
 'title': 'Live item ranks above the item it replaced',
 'lede': 'Found by the real Gemini eval: a retired note could rank above the live note that '
         'replaced it.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['1ef53fe'],
 'what': 'After loading hits, `rankBelowReplacers` puts each note one rank below its replacer when '
         'the replacer is a direct hit, along chains (v1 below v2 below v3).',
 'why': 'Query `upload limit` over 5 MB -> 10 MB -> 25 MB returned `[v2 v3 v1]` with Gemini: the '
        'meaning list ranked the 10 MB note first. Word search alone hid it by the accepted-first '
        'tie-break.',
 'flow': 'Recall                                 memory/recall.go\n'
         '  fuse words + meaning (RRF), expand links\n'
         '  load (now with replaced_by)          memory/recall.go:181\n'
         '  rankBelowReplacers                   memory/recall.go:119\n'
         '  sort, cut',
 'changes': [{'h': 'memory/recall.go',
              'text': ['`load` reads `replaced_by` into an unexported field; '
                       '`rankBelowReplacers`.'],
              'where': ['memory/recall.go:119']},
             {'h': 'memory/recall_test.go',
              'text': ['fake embedder ranks v1 closest; expect [3 2 1].']}],
 'decisions': [{'h': 'Only direct replacers',
                'text': ['A replacer reached only by a link already sits one rank below the note '
                         'it came from; changing that broke '
                         '`TestRecall_ExpansionReachesReplacedItem`.']}],
 'proof': [{'check': 'Fake embedder ranking retired first: [3 2 1]',
            'how': 'failed before: `ids = [1 2 3], want [3 2 1]`',
            'test': 'TestRecall_LiveBeforeItemItReplacedWhenMeaningRanksRetiredFirst'},
           {'check': 'Existing recall tests and eval pass',
            'how': 'go test ./memory/...',
            'test': '-'},
           {'check': 'Real Gemini eval',
            'how': '`replaced-chain-newest-first` passes; eval 30 scenarios, recall@10 1.000, mrr '
                   '1.000',
            'test': 'go test -tags embed ./memory/eval'}],
 'try': ['`go test -run LiveBefore -v ./memory`'],
 'left': [{'h': 'Superseded by #42',
           'text': ['#42 drops the "direct only" limit: a live replacer now ranks above the item '
                    'it replaced however either was reached.']}]}
