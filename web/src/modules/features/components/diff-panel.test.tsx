import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { DiffPanel } from './diff-panel';

const { useSendReview } = vi.hoisted(() => ({ useSendReview: vi.fn() }));

vi.mock('@/modules/features/api/send-review', () => ({
  useSendReview,
}));

describe('DiffPanel', () => {
  it('shows a file with no hunks instead of crashing', () => {
    useSendReview.mockReturnValue({ isPending: false, isError: false });

    const markup = renderToStaticMarkup(
      <DiffPanel files={[{ path: 'binary.bin', status: 'added', hunks: [] }]} featureId={1} />,
    );

    expect(markup).toContain('binary.bin');
    expect(markup).toContain('No text changes.');
  });

  it('keys each file by its path', () => {
    useSendReview.mockReturnValue({ isPending: false, isError: false });

    const markup = renderToStaticMarkup(
      <DiffPanel
        files={[
          { path: 'a.txt', status: 'added', hunks: [] },
          { path: 'b.txt', status: 'deleted', hunks: [] },
        ]}
        featureId={1}
      />,
    );

    expect(markup).toContain('a.txt');
    expect(markup).toContain('b.txt');
  });

  it('makes a deleted line clickable, keyed by its old line number', () => {
    useSendReview.mockReturnValue({ isPending: false, isError: false });

    const markup = renderToStaticMarkup(
      <DiffPanel
        files={[
          {
            path: 'gone.txt',
            status: 'deleted',
            hunks: [
              {
                header: '@@ -1 +0,0 @@',
                lines: [{ kind: 'del', old: 1, text: 'bye' }],
              },
            ],
          },
        ]}
        featureId={1}
      />,
    );

    expect(markup).toContain('data-line="1"');
    expect(markup).toContain('cursor-pointer');
  });
});
