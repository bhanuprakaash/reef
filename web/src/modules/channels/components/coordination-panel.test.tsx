import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Approval, DirectMessage, Turn } from '@/types/api';
import { groupChains } from '@/modules/channels/utils/direct-chains';
import { CoordinationPanel } from './coordination-panel';

const { useDecideApproval, useContinueChannel, useRetryTurn } = vi.hoisted(() => ({
  useDecideApproval: vi.fn(),
  useContinueChannel: vi.fn(),
  useRetryTurn: vi.fn(),
}));

vi.mock('@/lib/api/decide-approval', () => ({ useDecideApproval }));
vi.mock('@/lib/api/continue-channel', () => ({ useContinueChannel }));
vi.mock('@/lib/api/retry-turn', () => ({ useRetryTurn }));

process.env.TZ = 'UTC';

const request: DirectMessage = {
  id: 1,
  channelId: 2,
  chainId: 1,
  replyToId: null,
  sourceMessageId: null,
  from: 'developer',
  to: 'tester',
  body: 'Is the build green?',
  status: 'sent',
  createdAt: '2026-10-18T10:05:00Z',
};
const response: DirectMessage = {
  ...request,
  id: 2,
  replyToId: 1,
  from: 'tester',
  to: 'developer',
  body: 'Yes, all green',
  status: 'done',
  createdAt: '2026-10-18T10:07:00Z',
};
const none = { approvals: [], failedTurns: [], retryable: [], paused: false };

function render(
  messages: DirectMessage[],
  ctx: Partial<Parameters<typeof groupChains>[1]> = {},
  props: { running?: boolean; paused?: boolean } = {},
) {
  const [chain] = groupChains(messages, { ...none, ...ctx });
  return renderToStaticMarkup(
    <CoordinationPanel
      channelId={2}
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
    useDecideApproval.mockReturnValue({ isPending: false, isError: false });
    useContinueChannel.mockReturnValue({ isPending: false, isError: false });
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

  it('offers Approve and Reject while waiting approval', () => {
    const markup = render([request], { approvals: [{ id: 9, directMessageId: 1 } as Approval] });

    expect(markup).toContain('Approve');
    expect(markup).toContain('Reject');
    expect(markup).not.toContain('Continue');
    expect(markup).not.toContain('Retry');
  });

  it('offers Continue with the reason while paused', () => {
    const markup = render([request], { paused: true }, { paused: true });

    expect(markup).toContain('Paused: hop cap: 6 agent hops without a user message');
    expect(markup).toContain('Continue');
    expect(markup).not.toContain('Approve');
  });

  it('offers Retry for a failed or stopped chain with a retryable turn', () => {
    const failed: DirectMessage = { ...response, status: 'failed' };
    const turns = [{ id: 5, directMessageId: 1, error: 'boom' } as Turn];
    const retryable = { failedTurns: turns, retryable: turns };

    expect(render([request, failed], retryable)).toContain('Retry');
    expect(render([request, failed], retryable, { paused: true })).not.toContain('Retry');
    expect(render([request, failed], { failedTurns: turns })).not.toContain('Retry');
    expect(render([request, response])).not.toContain('Retry');
  });

  it('names a rejected chain and offers no action on it', () => {
    const rejected: DirectMessage = { ...response, status: 'failed', body: 'rejected' };
    const markup = render([request, rejected]);

    expect(markup).toContain('</span>rejected</span>');
    expect(markup).not.toContain('Retry');
    expect(markup).not.toContain('Approve');
  });

  it('shows the text and tool rows of the running direct turn, and nothing without one', () => {
    const [chain] = groupChains([request], { ...none, running: 'tester' });
    const live = {
      status: 'running' as const,
      direct: true,
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
          channelId={2}
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
