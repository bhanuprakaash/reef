PAGE = {'n': 34,
 'eyebrow': 'Write policy',
 'title': 'Convention authority and asking for approval',
 'lede': 'An architect or lead may still accept a convention. Any writer can now ask the user to '
         'approve a note first.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['cbbefa0'],
 'what': '`Note.NeedsApproval` makes the new item a candidate, whoever writes it. Without it, the '
         'rules of #24 stay: the user is accepted, a team-scope item from anyone else is a '
         'candidate, a convention is accepted only from architect or lead. The CLI flag is '
         '`--needs-approval`.',
 'why': 'R21 said every convention needs the user. Ticket #24 let architect and lead accept them, '
        'and no decision recorded that. R29 now records it and adds the opt-in, so a lead who is '
        'unsure of a rule can put it in front of the user.',
 'flow': 'reef memory add ... --needs-approval          cmd/reef/memory.go:163\n'
         '  Note{..., NeedsApproval: true}              cmd/reef/memory.go:184\n'
         '  Remember -> statusFor(w, n)                 memory/memory.go:321\n'
         '    n.NeedsApproval            -> candidate   (first rule)\n'
         '    w.Author == "user"         -> accepted\n'
         '    team scope                 -> candidate\n'
         '    convention, not arch/lead  -> candidate   memory/memory.go:71\n'
         '    else                       -> accepted',
 'changes': [{'h': 'memory/memory.go',
              'text': ['`Note` gets `NeedsApproval`. `statusFor` takes the whole note and checks '
                       'the flag first.'],
              'where': ['memory/memory.go:95', 'memory/memory.go:321']},
             {'h': 'cmd/reef/memory.go, cmd/reef/main.go',
              'text': ['`reef memory add` takes `--needs-approval`; usage lines name it.'],
              'where': ['cmd/reef/memory.go:163']},
             {'h': 'docs',
              'text': ['decisions.md R29. docs/memory.md add row says the flag makes a '
                       'candidate.']}],
 'decisions': [{'h': 'The flag wins over the user rule',
                'text': ['A user who writes with `--needs-approval` also gets a candidate. One '
                         'rule, checked first, with no exceptions.']}],
 'proof': [{'check': 'R29 recorded with a reason',
            'how': 'docs/restructure/decisions.md last row',
            'test': '-'},
           {'check': 'Architect and lead conventions accepted, developer candidate, any role with '
                     'the flag candidate',
            'how': '12 rows with literal statuses',
            'test': 'TestRemember_Status'},
           {'check': '`--needs-approval` gives a candidate that `pending` lists',
            'how': 'real binary: `2  candidate  convention  project:1  Wrap errors with %w`, then '
                   'pending lists it, approve gives `2  accepted`',
            'test': 'TestCLI_MemoryItems'},
           {'check': 'Docs name the flag', 'how': 'docs/memory.md add row', 'test': '-'}],
 'try': ['`reef memory add convention "Wrap errors with %w" --scope project:1 --needs-approval`',
         '`reef memory pending`, then `reef memory approve <id>`'],
 'left': [{'h': 'Agents cannot set the flag yet',
           'text': ['Agents cannot set the flag yet: the MCP `remember` tool is not wired '
                    '(R27).']}]}
