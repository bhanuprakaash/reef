import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, it, vi } from 'vitest';
import { BotPage } from './bot';

const hooks = vi.hoisted(() => ({
  pages: [] as unknown[][],
  latest: null as unknown,
  more: false,
  asked: [] as unknown[][],
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
vi.mock('@/lib/api/pins', () => ({
  pinKindOf: () => 'chat',
  useTogglePin: () => ({ pinned: false, toggle: { isPending: false, mutate: vi.fn() } }),
}));
vi.mock('@/modules/teams/api/get-bot-conversations', () => ({
  useBotChats: (id: number, projectId?: number) => {
    hooks.asked.push(['list', id, projectId]);
    return { isError: false, data: { pages: hooks.pages }, hasNextPage: hooks.more };
  },
  useBotLatestChat: (id: number, projectId?: number) => {
    hooks.asked.push(['latest', id, projectId]);
    return { data: hooks.latest };
  },
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

const entry = (spaceId: number) => ({
  kind: 'chat',
  spaceId,
  botId: 3,
  label: `chat ${spaceId}`,
  active: false,
  closed: false,
  lastActivity: '2026-10-31T10:00:00Z',
});

function render(stored: string) {
  vi.stubGlobal('localStorage', { getItem: () => stored, setItem: () => {} });
  hooks.asked = [];
  return renderToStaticMarkup(
    <MemoryRouter>
      <BotPage />
    </MemoryRouter>,
  );
}

afterEach(() => vi.unstubAllGlobals());

it('under a project asks for that project and resumes the latest chat it returns', () => {
  hooks.pages = [[entry(11), entry(10)]];
  hooks.latest = chat(11);

  const markup = render('1');

  expect(hooks.asked).toEqual([
    ['list', 3, 1],
    ['latest', 3, 1],
  ]);
  expect(markup).toContain('Resume chat');
  expect(markup).toContain('New chat');
});

it('under a project with no chat offers only New chat', () => {
  hooks.pages = [[]];
  hooks.latest = null;

  const markup = render('2');

  expect(markup).not.toContain('Resume chat');
  expect(markup).toContain('New chat');
});

it('under All projects asks without a project and lists every page once', () => {
  hooks.pages = [
    [entry(11), entry(10)],
    [entry(10), entry(9)],
  ];
  hooks.latest = chat(11);
  hooks.more = true;

  const markup = render('all');

  expect(hooks.asked).toEqual([
    ['list', 3, undefined],
    ['latest', 3, undefined],
  ]);
  expect(markup.match(/href="\/spaces\/10"/g)).toHaveLength(1);
  expect(markup).toContain('href="/spaces/9"');
  expect(markup).toContain('Load more');
});

it('labels each entry by kind and opens a Space or consultation as Bot activity', () => {
  hooks.pages = [
    [
      {
        ...entry(5),
        kind: 'consultation',
        label: 'Consultation from Prakash',
        active: true,
        origin: { spaceId: 2, label: 'Prakash · Go chat' },
      },
      { ...entry(6), kind: 'space', label: 'Ada · Login Space' },
    ],
  ];
  hooks.latest = null;
  hooks.more = false;

  const markup = render('all');

  expect(markup).toContain('href="/spaces/5/bots/3"');
  expect(markup).toContain('Consultation from Prakash');
  expect(markup).toContain('Asked from Prakash · Go chat');
  expect(markup).toContain('href="/spaces/6/bots/3"');
  expect(markup).toContain('Ada · Login Space');
  expect(markup).toContain('Working now');
});
