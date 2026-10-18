import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { Approval, DirectMessage, Turn } from '@/types/api';
import { groupChains } from '@/modules/channels/utils/direct-chains';
import { CoordinationItem } from './coordination-item';

const request: DirectMessage = {
  id: 1,
  channelId: 2,
  chainId: 1,
  replyToId: null,
  sourceMessageId: null,
  from: 'developer',
  to: 'tester',
  body: 'Secret body',
  status: 'sent',
  createdAt: '2026-10-18T10:00:00Z',
};
const response: DirectMessage = {
  ...request,
  id: 2,
  replyToId: 1,
  from: 'tester',
  to: 'developer',
  status: 'done',
};
const failed: DirectMessage = { ...response, status: 'failed' };
const none = { approvals: [], failedTurns: [], retryable: [], paused: false };

function render(messages: DirectMessage[], ctx: Partial<Parameters<typeof groupChains>[1]> = {}) {
  const [chain] = groupChains(messages, { ...none, ...ctx });
  return renderToStaticMarkup(
    <ul>
      <CoordinationItem chain={chain} faces={{}} open={false} onOpen={() => {}} />
    </ul>,
  );
}

describe('CoordinationItem', () => {
  it('is one button naming the pair, the count and the state, without any message body', () => {
    const markup = render([request], { running: 'tester' });

    expect(markup).toContain('developer and tester are coordinating · 1 message<');
    expect(markup).toContain('</span>working</span>');
    expect(markup.match(/<button/g)).toHaveLength(1);
    expect(markup).not.toContain('Secret body');
  });

  it('shows the new count and state when more messages arrive, still as one item', () => {
    const markup = render([request, response]);

    expect(markup).toContain('developer and tester coordinated · 2 messages<');
    expect(markup).toContain('</span>completed</span>');
    expect(markup.match(/<li[ >]/g)).toHaveLength(1);
  });

  it('tells waiting approval, paused, failed and stopped apart', () => {
    const approvals = [{ id: 9, directMessageId: 1 } as Approval];
    const interrupted = [{ id: 5, directMessageId: 1, error: 'interrupted' } as Turn];

    expect(render([request], { approvals })).toContain('</span>waiting approval</span>');
    expect(render([request], { paused: true })).toContain('</span>paused</span>');
    expect(render([request, failed])).toContain('</span>failed</span>');
    expect(render([request, failed], { failedTurns: interrupted })).toContain(
      '</span>stopped</span>',
    );
  });

  it('tells a rejected approval and a wake shed by a stop from a failure', () => {
    expect(render([request, { ...failed, body: 'rejected' }])).toContain('</span>rejected</span>');
    expect(render([request, { ...failed, body: 'stopped' }])).toContain('</span>stopped</span>');
  });
});
