import { expect, it } from 'vitest';
import { catalog, catalogEntryFor, catalogInput, searchCatalog } from './catalog';

const entry = (id: string) => catalog.find((e) => e.id === id)!;

it('searches the catalog by name and description', () => {
  expect(searchCatalog('GIT').map((e) => e.id)).toEqual(['gitlab', 'github']);
  expect(searchCatalog('confluence').map((e) => e.id)).toEqual(['atlassian']);
  expect(searchCatalog('nothing like this')).toEqual([]);
  expect(searchCatalog('  ')).toHaveLength(catalog.length);
});

it('builds the GitLab URL from the host, defaulting to gitlab.com', () => {
  expect(catalogInput(entry('gitlab'), { host: 'gitlab.example.com' })).toEqual({
    name: 'gitlab',
    url: 'https://gitlab.example.com/api/v4/mcp',
  });
  expect(catalogInput(entry('gitlab'), {})).toEqual({
    name: 'gitlab',
    url: 'https://gitlab.com/api/v4/mcp',
  });
});

it('adds the Sentry organisation to the path only when given', () => {
  expect(catalogInput(entry('sentry'), { organisation: 'acme' }).url).toBe(
    'https://mcp.sentry.dev/mcp/acme',
  );
  expect(catalogInput(entry('sentry'), { organisation: ' ' }).url).toBe(
    'https://mcp.sentry.dev/mcp',
  );
});

it('sends Slack with its own OAuth client id and callback port', () => {
  expect(catalogInput(entry('slack'), {})).toEqual({
    name: 'slack',
    url: 'https://mcp.slack.com/mcp',
    oauth: { clientId: '1601185624273.8899143856786', callbackPort: 3118 },
  });
});

it('sends the GitHub token as a bearer header and no OAuth', () => {
  expect(catalogInput(entry('github'), { token: 'ghp_abc' })).toEqual({
    name: 'github',
    url: 'https://api.githubcopilot.com/mcp/',
    headers: { Authorization: 'Bearer ghp_abc' },
  });
});

it('sends a typed client id as the OAuth client id', () => {
  const needsClientId = { ...entry('notion'), clientIdRequired: true };

  expect(catalogInput(needsClientId, { clientId: 'abc' }).oauth).toEqual({ clientId: 'abc' });
});

it('finds the catalog entry of a connection URL', () => {
  expect(catalogEntryFor('https://gitlab.example.com/api/v4/mcp')?.id).toBe('gitlab');
  expect(catalogEntryFor('https://mcp.sentry.dev/mcp/acme')?.id).toBe('sentry');
  expect(catalogEntryFor('https://docs.example/mcp')).toBeUndefined();
  expect(catalogEntryFor(undefined)).toBeUndefined();
});
