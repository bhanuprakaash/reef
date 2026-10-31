import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, it, vi } from 'vitest';
import { BotPage } from './bot';

const hooks = vi.hoisted(() => ({
  conversations: [] as unknown[],
  spaces: [] as unknown[],
}));

vi.mock('react-router', async (orig) => ({
  ...(await orig<typeof import('react-router')>()),
  useParams: () => ({ id: '3' }),
}));
vi.mock('@/lib/api/get-bots', () => ({
  useBots: () => ({
    isPending: false,
    isError: false,
    data: [{ id: 3, name: 'Ada', backend: 'claude', model: '', description: '' }],
  }),
}));
vi.mock('@/lib/api/get-teams', () => ({ useTeams: () => ({ data: [] }) }));
vi.mock('@/lib/api/get-memory', () => ({ useMemory: () => ({ data: [], isError: false }) }));
vi.mock('@/modules/projects/api/get-projects', () => ({
  useProjects: () => ({
    data: [
      { id: 1, name: 'reef', path: '/reef' },
      { id: 2, name: 'site', path: '/site' },
    ],
  }),
}));
vi.mock('@/lib/api/get-spaces', () => ({
  useSpaces: () => ({ data: hooks.spaces }),
}));
vi.mock('@/modules/teams/api/get-bot-conversations', () => ({
  useBotConversations: () => ({ isError: false, data: hooks.conversations }),
}));
vi.mock('@/modules/spaces/api/start-conversation', () => ({
  useStartConversation: () => ({ isPending: false, isError: false, mutate: vi.fn() }),
}));

const chat = (id: number, movedTo: number | null = null) => ({
  id,
  kind: 'conversation',
  projectId: 0,
  workItemId: null,
  name: `chat ${id}`,
  closedAt: null,
  movedTo,
  createdAt: '2026-10-31T10:00:00Z',
  lastMessage: null,
});

// The server lists a Bot's chats newest first: 11 is newer than 10.
function render(stored: string, conversations: unknown[], inProject: number[]) {
  vi.stubGlobal('localStorage', { getItem: () => stored, setItem: () => {} });
  hooks.conversations = conversations;
  hooks.spaces = inProject.map((id) => chat(id));
  return renderToStaticMarkup(
    <MemoryRouter>
      <BotPage />
    </MemoryRouter>,
  );
}

afterEach(() => vi.unstubAllGlobals());

it('under a project resumes its latest associated chat and lists only associated chats', () => {
  const markup = render('1', [chat(11), chat(10)], [10]);

  expect(markup).toContain('Resume chat');
  expect(markup).toContain('href="/spaces/10"');
  expect(markup).not.toContain('href="/spaces/11"');
  expect(markup).toContain('New chat');
});

it('under a project with no associated chat offers only New chat', () => {
  const markup = render('2', [chat(11), chat(10)], []);

  expect(markup).not.toContain('Resume chat');
  expect(markup).toContain('New chat');
});

it('under All projects resumes the latest chat and keeps older ones listed', () => {
  const markup = render('all', [chat(11), chat(10)], [10, 11]);

  expect(markup).toContain('Resume chat');
  expect(markup.match(/href="\/spaces\/11"/g)).toHaveLength(2);
  expect(markup).toContain('href="/spaces/10"');
});

it('does not resume a conversation that moved to another Space', () => {
  const markup = render('all', [chat(11, 12), chat(10)], [10, 11]);

  expect(markup.match(/href="\/spaces\/10"/g)).toHaveLength(2);
});
