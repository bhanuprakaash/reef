PAGE = {'n': 40,
 'eyebrow': 'Clean-up',
 'title': 'Small memory review fixes',
 'lede': 'A clear error for a repeated replaces id, embed.go in style order, and docs that match '
         'the build.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['46dcf32', '96f7014', 'b3ecb2c'],
 'what': '`validate` refuses `Replaces [1, 1]` with `invalid: memory 1 listed twice in replaces` '
         'before anything is written. embed.go declarations follow go-style order (only order '
         'changed). Restructure docs say R15–R30 and that memory is built but not yet called by '
         'agents.',
 'why': 'The old message was `already retired`, which pointed the user the wrong way.',
 'flow': 'Remember -> validate                  memory/memory.go:337\n'
         '  duplicate id in n.Replaces -> ErrInvalid   :354',
 'changes': [{'h': 'memory/memory.go',
              'text': ['duplicate check in `validate`.'],
              'where': ['memory/memory.go:354']},
             {'h': 'memory/embed.go', 'text': ['reordered; sorted-lines diff empty.']},
             {'h': 'docs/restructure', 'text': ['index.md and memory-and-retrieval.md.']}],
 'decisions': [],
 'proof': [{'check': 'Replaces [1,1] fails, nothing written',
            'how': 'failed before: `replace 1: invalid: memory 1 is already retired`',
            'test': 'TestRemember_ReplacesTwice'},
           {'check': 'embed.go order only',
            'how': '`diff <(sort old) <(sort new)` empty',
            'test': '-'},
           {'check': 'Docs R15–R30, built status',
            'how': 'index.md:53, memory-and-retrieval.md:3',
            'test': '-'}],
 'try': ['`reef memory add decision x --scope project:1 --replaces 1 --replaces 1`'],
 'left': []}
