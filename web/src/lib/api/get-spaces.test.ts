import { beforeEach, expect, it, vi } from 'vitest';
import { spacesQuery } from './get-spaces';

const { api } = vi.hoisted(() => ({ api: vi.fn(async () => []) }));
vi.mock('@/lib/api-client', () => ({ api }));

beforeEach(() => api.mockClear());

it('lists every Space under All projects', async () => {
  const query = spacesQuery();
  await query.queryFn();

  expect(query.queryKey).toEqual(['spaces']);
  expect(api).toHaveBeenCalledWith('/spaces');
});

it('asks the server for the Spaces associated with one project', async () => {
  const query = spacesQuery(7);
  await query.queryFn();

  expect(query.queryKey).toEqual(['spaces', 'project', 7]);
  expect(api).toHaveBeenCalledWith('/spaces?project=7');
});
