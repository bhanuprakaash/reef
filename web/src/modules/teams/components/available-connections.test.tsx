import { renderToStaticMarkup } from 'react-dom/server';
import { expect, it } from 'vitest';
import type { AvailableConnection } from '@/types/api';
import { AvailableRow } from './available-connections';

const render = (server: AvailableConnection) =>
  renderToStaticMarkup(<AvailableRow server={server} importing={false} onAdd={() => {}} />);

it('shows name, source, kind and command with an Add button', () => {
  const markup = render({
    source: 'claude user',
    name: 'files',
    kind: 'command',
    command: 'npx',
    args: ['-y', 'files-mcp'],
    added: false,
  });

  expect(markup).toContain('files');
  expect(markup).toContain('claude user · Local process · npx -y files-mcp');
  expect(markup).toContain('aria-label="Add files from claude user"');
});

it('shows the URL of an HTTP server', () => {
  const markup = render({
    source: 'claude plugin slack',
    name: 'slack',
    kind: 'url',
    url: 'https://mcp.slack.com/mcp',
    added: false,
  });

  expect(markup).toContain('claude plugin slack · HTTP · https://mcp.slack.com/mcp');
});

it('shows an added server as added with no Add button', () => {
  const markup = render({
    source: 'codex',
    name: 'docs',
    kind: 'url',
    url: 'https://docs.example/mcp',
    added: true,
    connectionId: 4,
  });

  expect(markup).toContain('Added');
  expect(markup).not.toContain('<button');
});
