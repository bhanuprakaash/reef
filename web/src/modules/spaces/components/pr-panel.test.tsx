import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { WorkItemPR } from '@/types/api';
import { PrPanel } from './pr-panel';

const { useWorkItemPR, useMergeWorkItemPR } = vi.hoisted(() => ({
  useWorkItemPR: vi.fn(),
  useMergeWorkItemPR: vi.fn(),
}));

vi.mock('@/modules/spaces/api/get-work-item-pr', () => ({ useWorkItemPR }));
vi.mock('@/modules/spaces/api/merge-work-item-pr', () => ({ useMergeWorkItemPR }));

const pr: WorkItemPR = {
  number: 12,
  title: 'Add login',
  state: 'OPEN',
  url: 'https://github.com/x/y/pull/12',
  isDraft: false,
  reviewDecision: 'APPROVED',
  base: 'main',
  head: 'work-item/4',
  canMerge: true,
  checks: [],
  body: 'Line one\nLine two',
  additions: 10,
  deletions: 3,
  files: [
    { path: 'a.go', additions: 7, deletions: 1 },
    { path: 'b.go', additions: 3, deletions: 2 },
  ],
};

function render(data: WorkItemPR) {
  useWorkItemPR.mockReturnValue({ isPending: false, isError: false, data: { pr: data } });
  useMergeWorkItemPR.mockReturnValue({ isPending: false, isError: false });
  return renderToStaticMarkup(
    <MemoryRouter>
      <PrPanel workItemId={4} onClose={() => {}} />
    </MemoryRouter>,
  );
}

describe('PrPanel', () => {
  it('shows the body with its line breaks, clamped to six lines', () => {
    const markup = render(pr);

    expect(markup).toMatch(
      /class="[^"]*whitespace-pre-wrap[^"]*line-clamp-6[^"]*">Line one\nLine two</,
    );
  });

  it('shows no body block for an empty body', () => {
    expect(render({ ...pr, body: '' })).not.toContain('whitespace-pre-wrap');
  });

  it('hides the Changes section when the PR has no files', () => {
    const markup = render({ ...pr, additions: 0, deletions: 0, files: [] });

    expect(markup).not.toContain('Changes');
  });

  it('shows the change totals and each file linking to the review page', () => {
    const markup = render(pr);

    expect(markup).toContain('+10 −3 · 2 files');
    for (const [path, add, del] of [
      ['a.go', 7, 1],
      ['b.go', 3, 2],
    ]) {
      expect(markup).toMatch(
        new RegExp(
          `<a [^>]*href="/work-items/4/review"[^>]*>(?:(?!</a>).)*${path}(?:(?!</a>).)*\\+${add}(?:(?!</a>).)*−${del}(?:(?!</a>).)*</a>`,
          's',
        ),
      );
    }
  });
});
