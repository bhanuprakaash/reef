import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { Role } from '@/types/api';
import { RolePanel } from './role-panel';

const role: Role = {
  botId: 1,
  name: 'developer',
  lead: false,
  write: true,
  backend: 'claude',
  permissionMode: '',
  description: 'Builds what the architect sketched.',
  skills: [],
  tools: ['Read', 'Grep'],
  connections: [],
};

describe('RolePanel', () => {
  it('shows the description, the tools as chips and a close button for a claude role', () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <RolePanel role={role} onChange={() => {}} onClose={() => {}}>
          {null}
        </RolePanel>
      </MemoryRouter>,
    );

    expect(markup).toContain('Builds what the architect sketched.');
    expect(markup).toContain('Tools <span class="text-xs text-muted">2</span>');
    expect(markup).toContain('Add tool');
    expect(markup).toContain('aria-label="Close role"');
  });

  it('shows the role provider, or PATH when it names none', () => {
    const providers = [{ name: 'work', kind: 'claude', detected: false, folder: '/w' }];
    const render = (r: Role) =>
      renderToStaticMarkup(
        <MemoryRouter>
          <RolePanel role={r} providers={providers} onChange={() => {}} onClose={() => {}}>
            {null}
          </RolePanel>
        </MemoryRouter>,
      );

    expect(render(role)).toContain('Default (PATH)');
    const markup = render({ ...role, provider: 'work' });
    expect(markup).toContain('aria-label="developer provider"');
    expect(markup).not.toContain('Default (PATH)');
  });

  it('shows the connections a Bot has as chips', () => {
    const connections = [
      { id: 1, name: 'files', command: 'files-mcp', createdAt: '' },
      { id: 2, name: 'web', url: 'http://127.0.0.1:9/mcp', createdAt: '' },
    ];
    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <RolePanel
          role={{ ...role, connections: [2] }}
          connections={connections}
          onChange={() => {}}
          onClose={() => {}}
        >
          {null}
        </RolePanel>
      </MemoryRouter>,
    );

    expect(markup).toContain('Connections <span class="text-xs text-muted">1</span>');
    expect(markup).toContain('aria-label="Remove web"');
    expect(markup).not.toContain('aria-label="Remove files"');
    expect(markup).toContain('Add connection');
  });

  it('hides the tools of a codex role', () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <RolePanel role={{ ...role, backend: 'codex' }} onChange={() => {}} onClose={() => {}}>
          {null}
        </RolePanel>
      </MemoryRouter>,
    );

    expect(markup).not.toContain('Tools');
  });
});
