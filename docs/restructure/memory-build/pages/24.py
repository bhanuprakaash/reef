PAGE = {'n': 24,
 'eyebrow': 'Write policy',
 'title': 'Write authority and candidate status',
 'lede': "The writer's role decides if a new item is accepted at once or waits as a candidate for "
         'the user.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['1643565'],
 'what': '`Remember` asks `statusFor` for the status of each write. The user is always accepted. A '
         'team-scope item from anyone else is a candidate. A convention is accepted only from the '
         'architect or the lead. Every other kind is accepted from any role. The status is on '
         '`Result.Item.Status`.',
 'why': 'Decision R21: approval stays where a wrong item changes how the team works. R25: team '
        'lessons are candidates. The rule is a short table in code, not config or prompting.',
 'flow': 'Remember(ctx, w, n)                                  memory/memory.go:138\n'
         '  statusFor(w, n.Kind)                               memory/memory.go:302\n'
         '    w.Author == "user"            -> accepted\n'
         '    w.Scope.Kind == team          -> candidate\n'
         '    _acceptedRoles[kind] lacks role -> candidate     memory/memory.go:67\n'
         '    else                          -> accepted\n'
         '  INSERT memory (status)',
 'changes': [{'h': 'memory/memory.go',
              'text': ['`_acceptedRoles` maps `convention` to architect and lead. `statusFor` '
                       'applies the three rules in order.'],
              'where': ['memory/memory.go:67', 'memory/memory.go:302']},
             {'h': 'memory/memory_test.go',
              'text': ['Table test `TestRemember_Status`, eight rows with literal statuses. The '
                       "accept test now writes a fact, because a developer's convention is a "
                       'candidate.']}],
 'decisions': [{'h': 'Who is the user',
                'text': ['`Writer.Author == "user"`. The CLI writes as author user; agents write '
                         'with their teammate name and role.']}],
 'proof': [{'check': 'Developer convention is candidate; architect and user accepted',
            'how': 'rows 1 to 3',
            'test': 'TestRemember_Status'},
           {'check': "Any role's decision is accepted",
            'how': 'developer and tester rows',
            'test': 'TestRemember_Status'},
           {'check': 'Finding accepted; team lesson candidate',
            'how': 'tester finding, lead team lesson rows',
            'test': 'TestRemember_Status'},
           {'check': 'Table test with literal statuses',
            'how': 'give/want fields, string literals',
            'test': 'TestRemember_Status'}],
 'try': ['`go test -run TestRemember_Status -v ./memory`'],
 'left': []}
