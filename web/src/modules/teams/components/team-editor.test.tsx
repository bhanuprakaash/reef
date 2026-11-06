import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { Role, Team } from '@/types/api';
import { TeamEditor } from './team-editor';

vi.mock('@/modules/teams/api/delete-team', () => ({
  useDeleteTeam: () => ({ isPending: false, isError: false }),
}));
vi.mock('@/modules/teams/api/update-team', () => ({
  useUpdateTeam: () => ({ isPending: false, isSuccess: false, isError: false }),
}));
vi.mock('@/lib/api/get-bots', () => ({ useBots: () => ({ data: [] }) }));

const role: Role = {
  botId: 1,
  name: 'developer',
  lead: true,
  write: true,
  backend: 'claude',
  permissionMode: '',
  description: 'Builds what the architect sketched.',
  skills: [],
  tools: ['Read', 'Grep'],
  connections: [],
};

describe('TeamEditor', () => {
  it('lists member Bots with an answers-first star, remove and add controls but no Save button', () => {
    const team: Team = { id: 1, name: 'solo', responsibility: '', roles: [role] };

    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <TeamEditor team={team} onDeleted={() => {}} onDuplicate={() => {}} />
      </MemoryRouter>,
    );

    expect(markup).toContain('Builds what the architect sketched.');
    expect(markup).toContain('aria-label="developer answers first"');
    expect(markup).toContain('Answers first');
    expect(markup).toContain('aria-label="Remove developer"');
    expect(markup).toContain('Add Bot');
    expect(markup).not.toContain('Save');
  });
});
