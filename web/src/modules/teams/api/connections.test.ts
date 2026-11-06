import { beforeEach, expect, it, vi } from 'vitest';
import { catalog } from '@/modules/teams/utils/catalog';
import { useAddCatalogConnection, useImportConnection, useLoginConnection } from './connections';

const { api } = vi.hoisted(() => ({
  api: vi.fn<(path: string, init: { method: string; body: string }) => Promise<{ id: number }>>(),
}));
vi.mock('@/lib/api-client', () => ({ api }));
vi.mock('@tanstack/react-query', () => ({
  useMutation: (options: unknown) => options,
  useQuery: (options: unknown) => options,
  useQueryClient: () => ({ invalidateQueries: () => Promise.resolve() }),
}));

const mutation = useImportConnection as unknown as () => {
  mutationFn: (input: { source: string; name: string; projectId?: number }) => Promise<unknown>;
};
const { mutationFn } = mutation();

beforeEach(() => {
  api.mockReset();
  api.mockResolvedValue({ id: 9 });
});

it('posts the source, name and project to import', async () => {
  await mutationFn({ source: 'claude user', name: 'files', projectId: 4 });

  expect(api.mock.calls[0]![0]).toBe('/connections/import');
  expect(api.mock.calls[0]![1].method).toBe('POST');
  expect(JSON.parse(api.mock.calls[0]![1].body)).toEqual({
    source: 'claude user',
    name: 'files',
    projectId: 4,
  });
});

it('posts no projectId without a current project', async () => {
  await mutationFn({ source: 'codex', name: 'docs' });

  expect(JSON.parse(api.mock.calls[0]![1].body)).toEqual({ source: 'codex', name: 'docs' });
});

it('signs a connection in with a POST to its login', async () => {
  const login = (
    useLoginConnection as unknown as () => { mutationFn: (id: number) => Promise<unknown> }
  )();

  await login.mutationFn(7);

  expect(api.mock.calls[0]![0]).toBe('/connections/7/login');
  expect(api.mock.calls[0]![1].method).toBe('POST');
  expect(api.mock.calls[0]![1].body).toBe('{}');
});

const addCatalog = (
  useAddCatalogConnection as unknown as () => {
    mutationFn: (input: {
      entry: (typeof catalog)[number];
      values: Record<string, string>;
    }) => Promise<unknown>;
  }
)();
const entry = (id: string) => catalog.find((e) => e.id === id)!;

it('adds GitLab on a host with a POST to connections and then signs in', async () => {
  await addCatalog.mutationFn({ entry: entry('gitlab'), values: { host: 'gitlab.example.com' } });

  expect(api.mock.calls.map((c) => [c[0], c[1].method])).toEqual([
    ['/connections', 'POST'],
    ['/connections/9/login', 'POST'],
  ]);
  expect(JSON.parse(api.mock.calls[0]![1].body)).toEqual({
    name: 'gitlab',
    url: 'https://gitlab.example.com/api/v4/mcp',
  });
});

it('adds GitHub with a bearer header and makes no login call', async () => {
  await addCatalog.mutationFn({ entry: entry('github'), values: { token: 'ghp_abc' } });

  expect(api.mock.calls.map((c) => c[0])).toEqual(['/connections']);
  expect(JSON.parse(api.mock.calls[0]![1].body)).toEqual({
    name: 'github',
    url: 'https://api.githubcopilot.com/mcp/',
    headers: { Authorization: 'Bearer ghp_abc' },
  });
});
