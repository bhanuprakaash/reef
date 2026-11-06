import { beforeEach, expect, it, vi } from 'vitest';
import { skillsQuery } from './get-skills';
import { updateSkill } from '@/modules/teams/api/skill-library';

const { api } = vi.hoisted(() => ({ api: vi.fn(async () => []) }));
vi.mock('@/lib/api-client', () => ({ api }));

beforeEach(() => api.mockClear());

it('lists library and user skills under All projects', async () => {
  await skillsQuery().queryFn();

  expect(api).toHaveBeenCalledWith('/skills');
});

it('adds the skills of one project', async () => {
  await skillsQuery(7).queryFn();

  expect(api).toHaveBeenCalledWith('/skills?project=7');
});

it('posts an update with a JSON body so the server accepts it', async () => {
  await updateSkill(3);

  expect(api).toHaveBeenCalledWith('/skills/3/update', { method: 'POST', body: '{}' });
});
