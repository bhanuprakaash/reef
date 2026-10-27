import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { MemoryItem } from '@/types/api';
import { MemoryList } from './memory-list';

function item(id: number): MemoryItem {
  return {
    id,
    kind: 'decision',
    body: `choice-${id}`,
    why: `reason-${id}`,
    scope: 'work_item:1',
    author: 'user',
    createdAt: '2026-10-01T09:00:00Z',
  };
}

function render(items: MemoryItem[]) {
  return renderToStaticMarkup(<MemoryList items={items} faces={{}} onRetire={() => {}} />);
}

describe('MemoryList', () => {
  it('shows the first 5 items as given, a Show all button and a Retire button each, with the why left out', () => {
    const markup = render([7, 6, 5, 4, 3, 2, 1].map(item));

    for (const id of [7, 6, 5, 4, 3]) expect(markup).toContain(`choice-${id}<`);
    expect(markup).not.toContain('choice-2<');
    expect(markup).toContain('Show all 7');
    expect(markup).not.toContain('reason-7');
    expect(markup.match(/>Retire</g)).toHaveLength(5);
    expect(markup).toContain('aria-label="Retire decision 7"');
  });

  it('has no Show all for 5 or fewer items', () => {
    expect(render([3, 2, 1].map(item))).not.toContain('Show all');
  });

  it('says nothing remembered yet only when there are no items', () => {
    expect(render([])).toContain('Nothing remembered yet');
    expect(render([item(1)])).not.toContain('Nothing remembered yet');
  });
});
