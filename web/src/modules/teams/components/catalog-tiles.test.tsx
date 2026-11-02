import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { expect, it } from 'vitest';
import type { Connection } from '@/types/api';
import { CatalogTiles } from './catalog-tiles';

const render = (connections: Connection[]) =>
  renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <CatalogTiles connections={connections} />
    </QueryClientProvider>,
  );

it('shows Add on every service when nothing is connected', () => {
  const markup = render([]);

  expect(markup).toContain('aria-label="Add GitLab"');
  expect(markup).toContain('aria-label="Add Slack"');
  expect(markup).not.toContain('Added');
});

it('shows a service whose URL is already connected as Added with no Add', () => {
  const markup = render([
    { id: 3, name: 'slack', url: 'https://mcp.slack.com/mcp', createdAt: '' },
  ]);

  expect(markup).toContain('Added');
  expect(markup).not.toContain('aria-label="Add Slack"');
  expect(markup).toContain('aria-label="Add GitLab"');
});
