import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { FileDiff } from '@/types/api';
import { DiffPanel } from './diff-panel';

const { useSendReview } = vi.hoisted(() => ({ useSendReview: vi.fn() }));

vi.mock('@/modules/features/api/send-review', () => ({
  useSendReview,
}));

function render(files: FileDiff[]) {
  useSendReview.mockReturnValue({ isPending: false, isError: false });
  return renderToStaticMarkup(
    <MemoryRouter>
      <DiffPanel files={files} featureId={1} featureName="Login" branch="feature/login" />
    </MemoryRouter>,
  );
}

describe('DiffPanel', () => {
  it('shows a file with no hunks instead of crashing', () => {
    const markup = render([{ path: 'binary.bin', status: 'added', hunks: [] }]);

    expect(markup).toContain('binary.bin');
    expect(markup).toContain('No text changes.');
  });

  it('lists each file with its letter and strikes through a deleted one', () => {
    const markup = render([
      { path: 'a.txt', status: 'added', hunks: [] },
      { path: 'b.txt', status: 'deleted', hunks: [] },
    ]);

    expect(markup).toMatch(/>A<\/span><span class="[^"]*">a\.txt/);
    expect(markup).toMatch(/>D<\/span><span class="[^"]*line-through">b\.txt/);
  });

  it('makes a deleted line clickable, keyed by its old line number', () => {
    const markup = render([
      {
        path: 'gone.txt',
        status: 'deleted',
        hunks: [{ header: '@@ -1 +0,0 @@', lines: [{ kind: 'del', old: 1, text: 'bye' }] }],
      },
    ]);

    expect(markup).toContain('data-line="1"');
    expect(markup).toContain('cursor-pointer');
  });
});
