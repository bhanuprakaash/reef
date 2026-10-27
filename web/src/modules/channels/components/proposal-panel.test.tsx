import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { Proposal } from '@/types/api';
import { ProposalPanel } from './proposal-panel';

const { useProposal, useDecideProposal } = vi.hoisted(() => ({
  useProposal: vi.fn(),
  useDecideProposal: vi.fn(),
}));
vi.mock('@/modules/channels/api/get-proposal', () => ({ useProposal }));
vi.mock('@/modules/channels/api/decide-proposal', () => ({ useDecideProposal }));

const proposal: Proposal = {
  id: 3,
  channelId: 1,
  role: 'lead',
  kind: 'wrap',
  brief: 'Login with sessions.',
  items: [
    {
      kind: 'decision',
      body: 'lock an account after 5 failed logins',
      why: 'slows down guessing',
      scope: 'work_item',
      replaces: [4, 5],
    },
    { kind: 'fact', body: 'login lives at /login', scope: 'project' },
  ],
  retire: [7, 8],
  status: 'pending',
  createdAt: '',
};

function render(pending: Proposal | undefined, decided?: object) {
  useProposal.mockReturnValue({ data: pending, isError: false });
  useDecideProposal.mockReturnValue({ data: decided, isPending: false, isError: false });
  return renderToStaticMarkup(<ProposalPanel channelId={1} featureId={2} faces={{}} />);
}

describe('ProposalPanel', () => {
  it('lists each item as kind (scope): body with its why and replaces, then the retired ids', () => {
    const markup = render(proposal);

    expect(markup).toContain('2 items');
    expect(markup).toContain('decision (work_item): lock an account after 5 failed logins');
    expect(markup).toContain('Why: slows down guessing');
    expect(markup).toContain('Replaces: 4, 5');
    expect(markup).toContain('fact (project): login lives at /login');
    expect(markup).toContain('retire memory items: 7, 8');
  });

  it('shows no item list for a proposal of the brief only', () => {
    const markup = render({ ...proposal, kind: 'brief', items: [], retire: [] });

    expect(markup).not.toContain('Memory items');
    expect(markup).not.toContain('retire memory items');
  });

  it('says how many items an approved wrap saved', () => {
    const markup = render(undefined, { ...proposal, status: 'approved', itemsAdded: 3 });

    expect(markup).toContain('approved: 3 items saved');
  });
});
