import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { DirectMessage, Turn } from '@/types/api';
import { groupChains } from '@/modules/spaces/utils/direct-chains';
import { CoordinationPanel } from './coordination-panel';

const { useContinueSpace, useRetryTurn } = vi.hoisted(() => ({
  useContinueSpace: vi.fn(),
  useRetryTurn: vi.fn(),
}));

vi.mock('@/lib/api/continue-space', () => ({ useContinueSpace }));
vi.mock('@/lib/api/retry-turn', () => ({ useRetryTurn }));

process.env.TZ = 'UTC';

const request: DirectMessage = {
  id: 1,
  spaceId: 2,
  chainId: 1,
  replyToId: null,
  sourceMessageId: null,
  from: 'developer',
  fromBotId: 1,
  to: 'tester',
  toBotId: 2,
  body: 'Is the build green?',
  status: 'sent',
  state: 'waiting',
  createdAt: '2026-10-18T10:05:00Z',
};
const response: DirectMessage = {
  ...request,
  id: 2,
  replyToId: 1,
  from: 'tester',
  fromBotId: 2,
  to: 'developer',
  toBotId: 1,
  body: 'Yes, all green',
  status: 'done',
  state: undefined,
  createdAt: '2026-10-18T10:07:00Z',
};
const none = { retryable: [], paused: false };

function render(
  messages: DirectMessage[],
  ctx: Partial<Parameters<typeof groupChains>[1]> = {},
  props: { running?: boolean; paused?: boolean } = {},
) {
  const [chain] = groupChains(messages, { ...none, ...ctx });
  return renderToStaticMarkup(
    <CoordinationPanel
      spaceId={2}
      chain={chain}
      faces={{}}
      running={props.running ?? false}
      paused={props.paused ?? false}
      reason="hop cap: 6 agent hops without a user message"
      onOpenSource={() => {}}
      onClose={() => {}}
    />,
  );
}

describe('CoordinationPanel', () => {
  beforeEach(() => {
    useContinueSpace.mockReturnValue({ isPending: false, isError: false });
    useRetryTurn.mockReturnValue({ isPending: false, isError: false });
  });

  it('lists every message in order with sender, recipient, time and delivery status', () => {
    const markup = render([request, response]);

    expect(markup).toContain('aria-label="Coordination"');
    expect(markup).toContain('>developer → tester</span>');
    expect(markup).toContain('10:05');
    expect(markup).toContain(' · sent');
    expect(markup).toContain('>tester → developer</span>');
    expect(markup).toContain('10:07');
    expect(markup).toContain(' · done');
    expect(markup.indexOf('Is the build green?')).toBeLessThan(markup.indexOf('Yes, all green'));
  });

  it('links to the public message the chain began from', () => {
    const linked = render([{ ...request, sourceMessageId: 7 }, response]);
    const unlinked = render([request, response]);

    expect(linked).toContain('Open the message that started it');
    expect(unlinked).not.toContain('Open the message that started it');
  });

  it('offers Continue with the reason while paused', () => {
    const markup = render([request], { paused: true }, { paused: true });

    expect(markup).toContain('Paused: hop cap: 6 agent hops without a user message');
    expect(markup).toContain('Continue');
  });

  it('offers Retry for a failed or stopped chain with a retryable turn', () => {
    const failed: DirectMessage = { ...request, state: 'failed' };
    const turns = [{ id: 5, directMessageId: 1, error: 'boom' } as Turn];
    const retryable = { retryable: turns };

    expect(render([failed], retryable)).toContain('Retry');
    expect(render([failed], retryable, { paused: true })).not.toContain('Retry');
    expect(render([failed])).not.toContain('Retry');
    expect(render([request, response])).not.toContain('Retry');
  });

  it('shows the text and tool rows of the running direct turn, and nothing without one', () => {
    const [chain] = groupChains([request], { ...none, running: 2 });
    const live = {
      status: 'running' as const,
      directMessageId: 1,
      items: [
        { type: 'text' as const, text: 'Checking the build' },
        {
          type: 'tool' as const,
          tool: {
            id: 't1',
            name: 'Bash',
            input: { command: 'npm test' },
            done: false,
            output: '',
            isError: false,
            children: [],
          },
        },
      ],
    };
    const html = (turn?: typeof live) =>
      renderToStaticMarkup(
        <CoordinationPanel
          spaceId={2}
          chain={chain}
          live={turn}
          faces={{}}
          running
          paused={false}
          reason=""
          onOpenSource={() => {}}
          onClose={() => {}}
        />,
      );

    expect(html(live)).toContain('Checking the build');
    expect(html(live)).toContain('npm test');
    expect(html()).not.toContain('Checking the build');
  });
});
