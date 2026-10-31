import { beforeEach, expect, it, vi } from 'vitest';
import { useStartConversation } from './start-conversation';

const { api } = vi.hoisted(() => ({
  api: vi.fn<(path: string, init: { body: string }) => Promise<{ id: number }>>(),
}));
vi.mock('@/lib/api-client', () => ({ api }));
vi.mock('@tanstack/react-query', () => ({
  useMutation: (options: unknown) => options,
  useQueryClient: () => ({}),
}));

const bot = { id: 3, name: 'Ada' };
const mutation = useStartConversation as unknown as () => {
  mutationFn: (input: { projectId?: number; bot: typeof bot }) => Promise<unknown>;
};
const { mutationFn } = mutation();
const posted = () => JSON.parse(api.mock.calls[0]![1].body);

beforeEach(() => {
  api.mockReset();
  api.mockResolvedValue({ id: 9 });
});

it('posts no projectId under All projects', async () => {
  await mutationFn({ bot });

  expect(api.mock.calls[0]![0]).toBe('/spaces');
  expect(posted()).toEqual({
    name: expect.stringMatching(/^Ada /),
    botId: 3,
    kind: 'conversation',
  });
});

it('posts the projectId under a project', async () => {
  await mutationFn({ projectId: 4, bot });

  expect(posted()).toMatchObject({ projectId: 4, botId: 3, kind: 'conversation' });
});

it('creates a new Space on every call', async () => {
  await mutationFn({ projectId: 4, bot });
  await mutationFn({ projectId: 4, bot });

  expect(api).toHaveBeenCalledTimes(2);
});
