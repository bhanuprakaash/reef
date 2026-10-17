import { describe, expect, it } from 'vitest';
import { fileLanguage, findFileRefs, parseFileRef } from './file-ref';

describe('parseFileRef', () => {
  it.each([
    ['web/src/app.tsx', { path: 'web/src/app.tsx' }],
    ['web/src/app.tsx:42', { path: 'web/src/app.tsx', line: 42 }],
    ['./a/b.go:10-20', { path: './a/b.go', line: 10 }],
    ['src/x.ts#L42', { path: 'src/x.ts', line: 42 }],
    ['src/x.ts#L42-L50', { path: 'src/x.ts', line: 42 }],
    ['server/http.go:12:5', { path: 'server/http.go', line: 12 }],
    ['../docs/product.md', { path: '../docs/product.md' }],
    ['/Users/me/reef/go.mod', { path: '/Users/me/reef/go.mod' }],
    ['node_modules/@types/react/index.d.ts', { path: 'node_modules/@types/react/index.d.ts' }],
    ['README.md', { path: 'README.md' }],
    ['CLAUDE.md', { path: 'CLAUDE.md' }],
    ['Makefile', { path: 'Makefile' }],
    ['deploy/Dockerfile', { path: 'deploy/Dockerfile' }],
    ['package.JSON', { path: 'package.JSON' }],
    ['web/src/app.tsx:0', { path: 'web/src/app.tsx' }],
  ])('reads %s', (text, want) => {
    expect(parseFileRef(text)).toEqual(want);
  });

  it.each([
    'v1.2.3',
    'e.g.',
    '1.5x',
    'example.com',
    'http://x.io/a.go',
    'https://github.com/a/b.ts',
    'localhost:8080/a.ts',
    'me@x.io',
    'me@x.md',
    'npm run build',
    'src/**/*.ts',
    'src/app',
    'image.png',
    '.ts',
    'web/src/app.tsx:',
    'a.ts:x',
    'fmt.Println("hi")',
    '',
  ])('rejects %s', (text) => {
    expect(parseFileRef(text)).toBeNull();
  });
});

describe('findFileRefs', () => {
  it('finds bare paths with a slash and leaves trailing punctuation out', () => {
    const text = 'Edit web/src/app.tsx:42, then (see store/db.go). Done: docs/a.md.';
    const refs = findFileRefs(text);

    expect(refs.map((r) => text.slice(r.start, r.end))).toEqual([
      'web/src/app.tsx:42',
      'store/db.go',
      'docs/a.md',
    ]);
    expect(refs[0]).toMatchObject({ path: 'web/src/app.tsx', line: 42 });
  });

  it('ignores names without a slash, versions, urls, emails and abbreviations', () => {
    expect(
      findFileRefs(
        'README.md and app.tsx at v1.2.3, e.g. 1.5x faster; http://x.io/a.go me@x.io 2026/10/17',
      ),
    ).toEqual([]);
  });
});

describe('fileLanguage', () => {
  it.each([
    ['web/src/app.tsx', 'tsx'],
    ['a/b.go', 'go'],
    ['CLAUDE.md', 'markdown'],
    ['deploy/Dockerfile', 'dockerfile'],
    ['go.sum', ''],
    ['notes', ''],
  ])('%s is %s', (path, want) => {
    expect(fileLanguage(path)).toBe(want);
  });
});
