PAGE = {'n': 43,
 'eyebrow': 'Embedding settings',
 'title': 'Embedding settings saved together and owner-only first',
 'lede': 'Provider and key are saved in one transaction, and reef.db is made owner-only before the '
         'key is written.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['d81c188'],
 'what': '`store.SetSettings` writes several settings in one transaction and replaces '
         '`SetSetting`. `memoryEmbedSet` calls `restrictDB` before it, so the key never lands in a '
         'file others can read.',
 'why': "Two separate writes could leave the new provider with the old vendor's key, which would "
        'then be sent to the wrong vendor. The chmod ran after the key was already in a 0644 file.',
 'flow': 'reef memory embed <p> --key K          cmd/reef/memory.go\n'
         '  ClearVectors if provider changed     cmd/reef/memory.go:454\n'
         '  restrictDB (0600)                    cmd/reef/memory.go:464\n'
         '  SetSettings{provider, key} one tx    store/setting.go:25\n'
         '  Backfill',
 'changes': [{'h': 'store/setting.go',
              'text': ['`SetSettings(ctx, map)` in one transaction.'],
              'where': ['store/setting.go:25']},
             {'h': 'cmd/reef/memory.go',
              'text': ['chmod first, one save.'],
              'where': ['cmd/reef/memory.go:464']}],
 'decisions': [{'h': 'Map, not two named args',
                'text': ['Nothing else used `SetSetting`; one method covers both.']}],
 'proof': [{'check': 'A failed save leaves provider and key as they were',
            'how': 'failed before: `provider/key after failed switch = "voyage/sk-old", want '
                   '"openai/sk-old"`',
            'test': 'TestCLI_MemoryEmbedSavesProviderAndKeyTogether'},
           {'check': 'reef.db 0600 before the key is written',
            'how': 'failed before: `reef.db mode after failed save = 644, want 600`',
            'test': 'TestCLI_MemoryEmbedSavesProviderAndKeyTogether'},
           {'check': 'Existing embed tests', 'how': 'pass', 'test': 'TestCLI_MemoryEmbed*'}],
 'try': ['`reef memory embed gemini --key K`, `ls -l reef.db`'],
 'left': [{'h': 'No chmod-failure test',
           'text': ['No honest way to make chmod fail on an open db; by the code its error returns '
                    'before the save.']}]}
