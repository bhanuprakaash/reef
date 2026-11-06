PAGE = {'n': 39,
 'eyebrow': 'Security',
 'title': 'Owner-only reef.db when a key is stored',
 'lede': 'Once reef.db holds an embedding key, only its owner can read it.',
 'branch': 'memory-service',
 'base': 'docs/restructure-proposal',
 'commits': ['6671554'],
 'what': 'After `reef memory embed <provider> --key K` saves the key, `restrictDB` sets reef.db '
         'and any -wal/-shm beside it to 0600. SQLite creates later -wal/-shm files with the main '
         'file mode.',
 'why': 'reef.db was 0644, so any local user could read the key.',
 'flow': 'memoryEmbedSet -> SetSetting(key)      cmd/reef/memory.go\n'
         '  key != "" -> restrictDB             cmd/reef/memory.go:470\n'
         '    pragma_database_list main file -> chmod 0600 (+ -wal, -shm)   :492',
 'changes': [{'h': 'cmd/reef/memory.go',
              'text': ['`restrictDB` and its one call.'],
              'where': ['cmd/reef/memory.go:492']},
             {'h': 'docs/memory.md', 'text': ['embed row says the key makes reef.db owner-only.']}],
 'decisions': [{'h': 'In the CLI, not store/',
                'text': ['Only the key makes the file secret; store/ stays untouched.']}],
 'proof': [{'check': 'reef.db (and wal/shm) 0600 after embed',
            'how': 'failed before: `reef.db mode = 644, want 600`',
            'test': 'TestCLI_MemoryEmbedKeyMakesDBOwnerOnly'},
           {'check': 'Real binary and serve',
            'how': '`-rw------- reef.db`; reef serve opens it, /projects 200, new -wal/-shm also '
                   '0600',
            'test': 'real binary'},
           {'check': 'Docs', 'how': 'docs/memory.md embed row', 'test': '-'}],
 'try': ['`reef memory embed gemini --key K`, then `ls -l ~/.reef/reef.db` (or your --data dir)'],
 'left': [{'h': 'reef', 'text': ['reef.lock stays 0644; it holds no secret.']}]}
