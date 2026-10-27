import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { DirectMessage, Message } from '@/types/api';
import { groupChains } from '@/modules/spaces/utils/direct-chains';
import { groupThreads } from '@/modules/spaces/utils/group-threads';
import { MessageList } from './message-list';

const message = (id: number, author: string, body: string, createdAt: string): Message => ({
  id,
  spaceId: 2,
  parentId: null,
  author,
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
  to: 'tester',
  body: 'Secret private body',
  status: 'sent',
  createdAt: '2026-10-18T10:01:00Z',
};

function render(direct: DirectMessage[]) {
  const chains = groupChains(direct, {
    approvals: [],
    failedTurns: [],
    retryable: [],
    paused: false,
  });
  return renderToStaticMarkup(
    <MessageList
      threads={groupThreads(messages)}
      chains={chains}
      faces={{}}
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
});
