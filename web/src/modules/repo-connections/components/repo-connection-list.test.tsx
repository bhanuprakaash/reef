import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { expect, it } from 'vitest';
import { repoConnectionsKey } from '@/lib/query-keys';
import type { RepoConnection } from '@/types/api';
import { RepoConnectionList } from './repo-connection-list';

it('shows the account of a connected provider and the reason of a disconnected one', () => {
  const client = new QueryClient();
  const gitlab: RepoConnection = {
    id: 1,
    provider: 'gitlab',
    host: 'gitlab.com',
    account: 'ada',
    state: 'connected',
    reason: '',
    checkedAt: '',
  };
  client.setQueryData(repoConnectionsKey, [
    gitlab,
    {
      ...gitlab,
      id: 2,
      provider: 'github',
      host: 'github.com',
      account: '',
      state: 'disconnected',
      reason: 'login rejected: the token is expired or revoked',
    },
  ]);

  const markup = renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <RepoConnectionList />
    </QueryClientProvider>,
  );

  expect(markup).toContain('Connected as ada');
  expect(markup).toContain('Disconnected: login rejected: the token is expired or revoked');
  expect(markup.match(/Re-check/g)).toHaveLength(2);
});
