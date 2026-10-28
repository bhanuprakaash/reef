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
vi.mock('@/lib/api/get-teammates', () => ({ useTeammates: () => ({ data: [] }) }));
vi.mock('@/modules/teams/api/update-team', () => ({ useUpdateTeam }));

const role: Role = {
  teammateId: 1,
  name: 'developer',
  lead: false,
  write: true,
  backend: 'claude',
  permissionMode: '',
  persona: 'Builds what the architect sketched.',
  skills: [],
  tools: ['Read', 'Grep'],
  mcp: [],
};

describe('RolePanel', () => {
  it('shows the persona, the tool count and a close button for a claude role', () => {
    const markup = renderToStaticMarkup(
      <RolePanel scope="teammate" role={role} onChange={() => {}} onClose={() => {}}>
        {null}
      </RolePanel>,
    );

    expect(markup).toContain('Builds what the architect sketched.');
    expect(markup).toContain('2 allowed');
    expect(markup).toContain('aria-label="Close role"');
  });

  it('shows the role provider, or PATH when it names none', () => {
    const providers = [{ name: 'work', kind: 'claude', detected: false, folder: '/w' }];
    const render = (r: Role) =>
      renderToStaticMarkup(
        <RolePanel
          scope="teammate"
          role={r}
          providers={providers}
          onChange={() => {}}
          onClose={() => {}}
        >
          {null}
        </RolePanel>,
      );

    expect(render(role)).toContain('Default (PATH)');
    const markup = render({ ...role, provider: 'work' });
    expect(markup).toContain('aria-label="developer provider"');
    expect(markup).not.toContain('Default (PATH)');
  });

  it('hides the tools of a codex role', () => {
    const markup = renderToStaticMarkup(
      <RolePanel
        scope="teammate"
        role={{ ...role, backend: 'codex' }}
        onChange={() => {}}
        onClose={() => {}}
      >
        {null}
      </RolePanel>,
    );

    expect(markup).not.toContain('Tools');
  });

  it('leaves the persona and the model to the teammate when it shows a team membership', () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <RolePanel scope="member" role={role} onChange={() => {}} onClose={() => {}}>
          {null}
        </RolePanel>
      </MemoryRouter>,
    );

    expect(markup).not.toContain('Builds what the architect sketched.');
    expect(markup).not.toContain('developer model');
    expect(markup).toContain('aria-label="developer lead"');
    expect(markup).toContain('2 allowed');
  });
});

describe('TeamEditor', () => {
  it('lists the roles and opens no panel until one is picked', () => {
    useUpdateTeam.mockReturnValue({ isPending: false, isSuccess: false, isError: false });
    const team: Team = { id: 1, name: 'solo', responsibility: '', roles: [role] };

    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <TeamEditor team={team} onSaved={() => {}} onDeleted={() => {}} onDuplicate={() => {}} />
      </MemoryRouter>,
    );

    expect(markup).toContain('2 tools');
    expect(markup).not.toContain('aria-label="Role"');
  });

  it('opens the panel of the role named by openRole', () => {
    useUpdateTeam.mockReturnValue({ isPending: false, isSuccess: false, isError: false });
    const team: Team = {
      id: 1,
      name: 'solo',
      responsibility: '',
      roles: [{ ...role, name: 'lead' }, role],
    };
    const render = (openRole: string) =>
      renderToStaticMarkup(
        <MemoryRouter>
          <TeamEditor
            team={team}
            openRole={openRole}
            onSaved={() => {}}
            onDeleted={() => {}}
            onDuplicate={() => {}}
          />
        </MemoryRouter>,
      );

    expect(render('developer')).toContain('aria-label="developer name"');
    expect(render('developer')).not.toContain('aria-label="lead name"');
    expect(render('nobody')).not.toContain('aria-label="Role"');
  });
});
