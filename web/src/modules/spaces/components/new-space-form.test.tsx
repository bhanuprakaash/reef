import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { expect, it, vi } from 'vitest';
import { NewSpaceForm } from './new-space-form';

vi.mock('@/modules/spaces/api/create-space', () => ({
  useCreateSpace: () => ({ isPending: false, isError: false }),
}));
vi.mock('@/lib/api/get-teams', () => ({
  useTeams: () => ({
    data: [
      { id: 1, name: 'solo', responsibility: '', roles: [{ name: 'lead', face: 'jellyfish' }] },
    ],
  }),
}));

it('asks for a name and a team, not a backend or permission mode', () => {
  const markup = renderToStaticMarkup(
    <MemoryRouter>
      <NewSpaceForm projectId={1} workItemId={null} onClose={() => {}} />
    </MemoryRouter>,
  );

  expect(markup).toContain('solo');
  expect(markup).not.toContain('Backend');
  expect(markup).not.toContain('Permission');
});
