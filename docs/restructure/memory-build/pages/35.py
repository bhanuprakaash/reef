PAGE = {'n': 35,
 'eyebrow': 'Embedding settings',
 'title': 'Provider switch survives a crash',
 'lede': 'A provider switch clears the old vectors before it saves the new provider.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['284eace'],
 'what': '`memoryEmbedSet` now calls `ClearVectors` first when the provider changes, then saves '
         'provider and key, then backfills. A re-run after a crash still sees the old provider, '
         'clears again and refills every item.',
 'why': 'Before, the provider was saved first. A crash between save and clear made the re-run '
        'think nothing changed. Old vectors stayed, `Backfill` skipped them (not NULL) and '
        '`scoreVectors` skipped them (wrong length): those notes lost meaning search for good.',
 'flow': 'reef memory embed voyage --key K      cmd/reef/memory.go:454\n'
         '  provider != prev ? ClearVectors      (before any save)\n'
         '  SetSetting(provider), SetSetting(key)\n'
         '  Backfill                             memory/embed.go',
 'changes': [{'h': 'cmd/reef/memory.go',
              'text': ['Clear moved before the two `SetSetting` calls; uses `memory.Open(st.DB(), '
                       'nil)`.'],
              'where': ['cmd/reef/memory.go:454']},
             {'h': 'cmd/reef/memory_test.go',
              'text': ['`TestCLI_MemoryEmbedSwitchRecovers`: a trigger makes the clear fail; the '
                       'saved provider stays; after recovery no old-size vector remains.']}],
 'decisions': [{'h': 'Switch to none also clears',
                'text': ['Vectors are unused under none; clearing is harmless and keeps one '
                         'rule.']}],
 'proof': [{'check': 'Failed backfill after clear, re-run embeds every item',
            'how': '500 then re-run: 2 items, all 12-byte vectors',
            'test': 'TestCLI_MemoryEmbedSwitchRecovers'},
           {'check': 'Failed clear leaves the saved provider',
            'how': 'failed before the fix: `provider after failed clear = "openai", want '
                   '"voyage"`; passes after',
            'test': 'TestCLI_MemoryEmbedSwitchRecovers'},
           {'check': 'Existing embed test', 'how': 'passes', 'test': 'TestCLI_MemoryEmbed'}],
 'try': ['`reef memory embed openai --key A`, then `reef memory embed voyage --key B`'],
 'left': []}
