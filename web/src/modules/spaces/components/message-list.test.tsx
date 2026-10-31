import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { DirectMessage, Message, Turn } from '@/types/api';
import { groupChains } from '@/modules/spaces/utils/direct-chains';
import { groupThreads } from '@/modules/spaces/utils/group-threads';
import { Creature, type Faces } from '@/components/creature';
import { MessageList } from './message-list';

const { useRollbackTurn } = vi.hoisted(() => ({
  useRollbackTurn: vi.fn(),
}));

vi.mock('@/modules/spaces/api/rollback-turn', () => ({ useRollbackTurn }));

const botIds: Record<string, number> = { developer: 1, tester: 2, lead: 3 };
const message = (id: number, author: string, body: string, createdAt: string): Message => ({
  id,
  spaceId: 2,
  parentId: null,
  author,
  botId: botIds[author] ?? null,
  body,
  createdAt,
});
const messages = [
  message(1, 'user', '@developer @tester check the build', '2026-10-18T10:00:00Z'),
  message(2, 'developer', 'On it', '2026-10-18T10:03:00Z'),
];
const direct: DirectMessage = {
  id: 1,
  spaceId: 2,
  chainId: 1,
  replyToId: null,
  sourceMessageId: 1,
  from: 'developer',
  fromBotId: 1,
  to: 'tester',
  toBotId: 2,
  body: 'Secret private body',
  status: 'sent',
  state: 'answered',
  createdAt: '2026-10-18T10:01:00Z',
};

const turn = (id: number, role: string, startedAt: string): Turn => ({
  id,
  spaceId: 2,
  role,
  botId: botIds[role],
  status: 'done',
  error: '',
  wakeText: '',
  retried: false,
  checkpoint: `refs/reef/checkpoints/${id}`,
  startedAt,
});

function render(
  direct: DirectMessage[],
  { turns = [], checkpoints = [] }: { turns?: Turn[]; checkpoints?: Turn[] } = {},
) {
  useRollbackTurn.mockReturnValue({ isPending: false, isError: false });
  const chains = groupChains(direct, {
    retryable: [],
    paused: false,
  });
  return renderToStaticMarkup(
    <MessageList
      spaceId={2}
      threads={groupThreads(messages)}
      chains={chains}
      faces={{}}
      turns={turns}
      checkpoints={checkpoints}
      onOpen={() => {}}
      onOpenChain={() => {}}
    />,
  );
}

describe('MessageList', () => {
  it('shows a direct exchange as one item between the messages around it, with no bubble', () => {
    const markup = render([direct, { ...direct, id: 2, chainId: 1, replyToId: 1, status: 'done' }]);

    expect(markup.match(/coordinated · 2 messages/g)).toHaveLength(1);
    expect(markup).not.toContain('Secret private body');
    expect(markup.indexOf('check the build')).toBeLessThan(markup.indexOf('coordinated'));
    expect(markup.indexOf('coordinated')).toBeLessThan(markup.indexOf('On it'));
  });

  it('keeps a public mention of several roles as one ordinary bubble and no item', () => {
    const markup = render([]);

    expect(markup.match(/check the build/g)).toHaveLength(1);
    expect(markup).not.toContain('coordinat');
  });

  it('offers reply as a hover icon on a bubble with no thread', () => {
    const markup = render([]);

    expect(markup).toContain('aria-label="Reply in thread"');
    expect(markup).not.toContain('>Reply<');
  });

  it('keeps the thread count as text on a bubble with replies', () => {
    const root = message(1, 'user', 'check the build', '2026-10-18T10:00:00Z');
    const reply = { ...message(2, 'developer', 'On it', '2026-10-18T10:03:00Z'), parentId: 1 };
    const chains = groupChains([], { retryable: [], paused: false });
    const markup = renderToStaticMarkup(
      <MessageList
        spaceId={2}
        threads={groupThreads([root, reply])}
        chains={chains}
        faces={{}}
        turns={[]}
        checkpoints={[]}
        onOpen={() => {}}
        onOpenChain={() => {}}
      />,
    );

    expect(markup).toContain('1 reply in thread');
  });

  it('hangs the rollback icon on the last bubble of the checkpointed turn', () => {
    const convo = [
      message(1, 'user', 'go', '2026-10-18T10:00:00Z'),
      message(2, 'developer', 'made it', '2026-10-18T10:03:00Z'),
      message(3, 'user', 'again', '2026-10-18T11:00:00Z'),
      message(4, 'developer', 'made it again', '2026-10-18T11:05:00Z'),
    ];
    const turns = [
      turn(7, 'developer', '2026-10-18T10:02:00Z'),
      turn(9, 'developer', '2026-10-18T11:04:00Z'),
    ];
    const chains = groupChains([], { retryable: [], paused: false });
    const markup = renderToStaticMarkup(
      <MessageList
        spaceId={2}
        threads={groupThreads(convo)}
        chains={chains}
        faces={{}}
        turns={turns}
        checkpoints={[turns[0]]}
        onOpen={() => {}}
        onOpenChain={() => {}}
      />,
    );

    expect(markup.match(/title="Before turn 7/g)).toHaveLength(1);
    expect(markup).not.toContain('Before turn 9');
  });

  describe('author identity', () => {
    const jellyfish = renderToStaticMarkup(<Creature look={{ face: 'jellyfish' }} size={20} />);
    const neutral = renderToStaticMarkup(<Creature size={20} />);
    const faces: Faces = { 1: { face: 'jellyfish' } };
    const byAuthor = (author: string, botId: number | null, faces: Faces) =>
      renderToStaticMarkup(
        <MessageList
          spaceId={2}
          threads={groupThreads([
            { ...message(1, author, 'hello', '2026-10-18T10:00:00Z'), botId },
          ])}
          chains={[]}
          faces={faces}
          turns={[]}
          checkpoints={[]}
          onOpen={() => {}}
          onOpenChain={() => {}}
        />,
      );

    it('keeps the avatar of a renamed Bot and the recorded author text', () => {
      const markup = byAuthor('old-name', 1, faces);

      expect(markup).toContain(jellyfish);
      expect(markup).toContain('>old-name<');
    });

    it('gives a new Bot that reuses an old name a different avatar than the old Bot', () => {
      const markup = byAuthor('developer', 2, faces);

      expect(markup).not.toContain(jellyfish);
      expect(markup).toContain(neutral);
    });

    it('falls back to a neutral avatar when the message has no Bot id', () => {
      const markup = byAuthor('developer', null, { ...faces, developer: { face: 'jellyfish' } });

      expect(markup).not.toContain(jellyfish);
      expect(markup).toContain(neutral);
    });
  });
});
