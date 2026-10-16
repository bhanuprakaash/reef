import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { Role, Team } from '@/types/api';
import { RolePanel } from './role-panel';
import { TeamEditor } from './team-editor';

const { useUpdateTeam } = vi.hoisted(() => ({ useUpdateTeam: vi.fn() }));

vi.mock('@/modules/teams/api/update-team', () => ({ useUpdateTeam }));

const role: Role = {
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
      <RolePanel role={role} onChange={() => {}} onRemove={() => {}} onClose={() => {}} />,
    );

    expect(markup).toContain('Builds what the architect sketched.');
    expect(markup).toContain('2 allowed');
    expect(markup).toContain('aria-label="Close role"');
  });

  it('hides the tools of a codex role', () => {
    const markup = renderToStaticMarkup(
      <RolePanel
        role={{ ...role, backend: 'codex' }}
        onChange={() => {}}
        onRemove={() => {}}
        onClose={() => {}}
      />,
    );

    expect(markup).not.toContain('Tools');
  });
});

describe('TeamEditor', () => {
  it('lists the roles and opens no panel until one is picked', () => {
    useUpdateTeam.mockReturnValue({ isPending: false, isSuccess: false, isError: false });
    const team: Team = { name: 'solo', roles: [role] };

    const markup = renderToStaticMarkup(
      <TeamEditor team={team} onSaved={() => {}} onDuplicate={() => {}} />,
    );

    expect(markup).toContain('2 tools');
    expect(markup).not.toContain('aria-label="Role"');
  });
});
