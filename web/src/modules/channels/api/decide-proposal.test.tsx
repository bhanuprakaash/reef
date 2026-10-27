import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Proposal } from '@/types/api';
import { useDecideProposal } from './decide-proposal';

const proposal = { id: 3 } as Proposal;

async function approve(featureId: number | null) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ id: 3, status: 'approved' }))),
  );
  const client = new QueryClient();
  client.setQueryData(['memory', 'project:1'], []);
  client.setQueryData(['memory', 'work_item:2'], []);
  let decide!: ReturnType<typeof useDecideProposal>;
  function Probe() {
    decide = useDecideProposal(1, featureId);
    return null;
  }
  renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <Probe />
    </QueryClientProvider>,
  );
  await decide.mutateAsync({ proposal, action: 'approve' });
  return client;
}

afterEach(() => vi.unstubAllGlobals());

describe('useDecideProposal', () => {
  it('marks the project memory query stale after an approve', async () => {
    const client = await approve(2);

    expect(client.getQueryState(['memory', 'project:1'])?.isInvalidated).toBe(true);
    expect(client.getQueryState(['memory', 'work_item:2'])?.isInvalidated).toBe(true);
  });

  it('marks the project memory query stale after an approve in a quick session', async () => {
    const client = await approve(null);

    expect(client.getQueryState(['memory', 'project:1'])?.isInvalidated).toBe(true);
  });
});
