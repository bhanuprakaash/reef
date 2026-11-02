import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { Role, Team } from '@/types/api';
import { RolePanel } from './role-panel';
import { TeamEditor } from './team-editor';

const { useDeleteTeam, useUpdateTeam } = vi.hoisted(() => ({
  useDeleteTeam: vi.fn(() => ({ isPending: false, isError: false })),
  useUpdateTeam: vi.fn(),
}));

vi.mock('@/modules/teams/api/delete-team', () => ({ useDeleteTeam }));
vi.mock('@/modules/teams/api/add-member', () => ({
  useAddMember: () => ({ isPending: false, isError: false }),
}));
vi.mock('@/lib/api/get-bots', () => ({ useBots: () => ({ data: [] }) }));
vi.mock('@/modules/teams/api/update-team', () => ({ useUpdateTeam }));

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

describe('TeamEditor', () => {
  it('lists member Bots with coordinator suggestion and remove controls but no per-member settings', () => {
    useUpdateTeam.mockReturnValue({ isPending: false, isSuccess: false, isError: false });
    const team: Team = { id: 1, name: 'solo', responsibility: '', roles: [role] };

    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <TeamEditor team={team} onSaved={() => {}} onDeleted={() => {}} onDuplicate={() => {}} />
      </MemoryRouter>,
    );

    expect(markup).toContain('developer');
    expect(markup).toContain('aria-label="developer suggested coordinator"');
    expect(markup).toContain('Remove');
    expect(markup).not.toContain('aria-label="developer name"');
    expect(markup).not.toContain('aria-label="developer permission mode"');
  });
});
