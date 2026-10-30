import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { DirectMessage } from '@/types/api';
import { groupChains } from '@/modules/spaces/utils/direct-chains';
import { CoordinationItem } from './coordination-item';

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
  body: 'Secret body',
  status: 'sent',
  state: 'waiting',
  createdAt: '2026-10-18T10:00:00Z',
};
const response: DirectMessage = {
  ...request,
  id: 2,
  replyToId: 1,
  from: 'tester',
  fromBotId: 2,
  to: 'developer',
  toBotId: 1,
  status: 'done',
  state: undefined,
};
const answered: DirectMessage = { ...request, state: 'answered' };
const failed: DirectMessage = { ...request, state: 'failed' };
const none = { retryable: [], paused: false };

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
    const markup = render([request], { running: 2 });

    expect(markup).toContain('developer and tester are coordinating · 1 message<');
    expect(markup).toContain('</span>working</span>');
    expect(markup.match(/<button/g)).toHaveLength(1);
    expect(markup).not.toContain('Secret body');
  });

  it('shows the new count and state when more messages arrive, still as one item', () => {
    const markup = render([answered, response]);

    expect(markup).toContain('developer and tester coordinated · 2 messages<');
    expect(markup).toContain('</span>completed</span>');
    expect(markup.match(/<li[ >]/g)).toHaveLength(1);
  });

  it('tells paused, failed and stopped apart', () => {
    expect(render([request], { paused: true })).toContain('</span>paused</span>');
    expect(render([failed, response])).toContain('</span>failed</span>');
    expect(render([{ ...failed, state: 'stopped' }, response])).toContain('</span>stopped</span>');
  });
});
