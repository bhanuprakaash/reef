import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { Decision } from '@/types/api';
import { DecisionList } from './decision-list';

function decision(id: number, retiredAt: string | null = null): Decision {
  return {
    id,
    featureId: 1,
    messageId: null,
    body: `choice-${id}`,
    why: `reason-${id}`,
    author: 'user',
    createdAt: '2026-10-01T09:00:00Z',
    retiredAt,
    replacedBy: null,
  };
}

describe('DecisionList', () => {
  it('shows the newest 5 live decisions and a Show all button, with the why left out', () => {
    const decisions = [1, 2, 3, 4, 5, 6, 7].map((id) => decision(id));
    const markup = renderToStaticMarkup(<DecisionList decisions={decisions} faces={{}} />);

    for (const id of [3, 4, 5, 6, 7]) expect(markup).toContain(`choice-${id}<`);
    expect(markup).not.toContain('choice-2<');
    expect(markup).toContain('Show all 7');
    expect(markup).not.toContain('reason-7');
  });

  it('keeps retired decisions in their fold and no Show all for 5 or fewer live ones', () => {
    const decisions = [decision(1, '2026-10-02T09:00:00Z'), decision(2), decision(3)];
    const markup = renderToStaticMarkup(<DecisionList decisions={decisions} faces={{}} />);

    expect(markup).toContain('Retired (1)');
    expect(markup).not.toContain('Show all');
  });

  it('says no decisions yet only when there are none, retired or live', () => {
    const empty = renderToStaticMarkup(<DecisionList decisions={[]} faces={{}} />);
    const onlyRetired = renderToStaticMarkup(
      <DecisionList decisions={[decision(1, '2026-10-02T09:00:00Z')]} faces={{}} />,
    );

    expect(empty).toContain('No decisions yet');
    expect(onlyRetired).not.toContain('No decisions yet');
    expect(onlyRetired).toContain('Retired (1)');
  });
});
