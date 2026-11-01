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
      {
        id: 1,
        name: 'solo',
        responsibility: '',
        roles: [{ botId: 1, name: 'lead', face: 'jellyfish' }],
      },
    ],
  }),
}));

vi.mock('@/lib/api/get-bots', () => ({
  useBots: () => ({
    data: [
      { id: 1, name: 'lead', face: 'jellyfish' },
      { id: 2, name: 'xena', face: 'jellyfish' },
    ],
  }),
}));

it('asks for a name and Crews, not a backend or permission mode', () => {
  const markup = renderToStaticMarkup(
    <MemoryRouter>
      <NewSpaceForm projectId={1} workItemId={null} onClose={() => {}} />
    </MemoryRouter>,
  );

  expect(markup).toContain('solo');
  expect(markup).not.toContain('Backend');
  expect(markup).not.toContain('Permission');
});

it('preselects no Crew and no Bot', () => {
  const markup = renderToStaticMarkup(
    <MemoryRouter>
      <NewSpaceForm projectId={1} workItemId={null} onClose={() => {}} />
    </MemoryRouter>,
  );

  expect(markup).toContain('aria-pressed="false"');
  expect(markup).not.toContain('aria-pressed="true"');
  expect(markup).toContain('aria-label="Include lead"');
  expect(markup).not.toMatch(/aria-label="Include lead"[^>]*checked/);
  expect(markup).not.toContain('coordinates');
});

it('opens a Bot proposal with its Bots and coordinator, and no Crew chosen', () => {
  const proposal = { id: 1, proposedBy: 'lead', name: 'review', botIds: [2], coordinatorBotId: 2 };
  const markup = renderToStaticMarkup(
    <MemoryRouter>
      <NewSpaceForm workItemId={null} proposal={proposal} onClose={() => {}} />
    </MemoryRouter>,
  );

  expect(markup).toContain('value="review"');
  expect(markup).toContain('aria-label="Include xena"');
  expect(markup).toMatch(/aria-label="Include xena"[^>]*checked/);
  expect(markup).not.toMatch(/aria-label="Include lead"[^>]*checked/);
  expect(markup).toMatch(/aria-label="xena coordinates"[^>]*checked/);
});

it('uses the Work item owner and participants and requires a Space name', () => {
  const markup = renderToStaticMarkup(
    <MemoryRouter>
      <NewSpaceForm projectId={1} workItemId={7} onClose={() => {}} />
    </MemoryRouter>,
  );

  expect(markup).toContain('This Space uses the Work item’s owner and participants.');
  expect(markup).toContain('placeholder="required"');
  expect(markup).toContain('required=""');
  expect(markup).toMatch(/disabled=""[^>]*>Start<\/button>/);
  expect(markup).not.toContain('Crews');
  expect(markup).not.toContain('Include lead');
  expect(markup).not.toContain('Include xena');
});
