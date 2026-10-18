import { describe, expect, it } from 'vitest';
import type { Approval, DirectMessage, Turn } from '@/types/api';
import { groupChains } from './direct-chains';

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
  createdAt: '2026-10-18T10:00:00Z',
};
const response: DirectMessage = {
  ...request,
  id: 2,
  replyToId: 1,
  from: 'tester',
  to: 'developer',
  body: 'Yes',
  status: 'done',
};
const failed: DirectMessage = { ...response, status: 'failed', body: '' };
const none = { approvals: [], failedTurns: [], retryable: [], paused: false };

const approval = { id: 9, directMessageId: 1 } as Approval;
const turn = (error: string) => ({ id: 5, directMessageId: 1, error }) as Turn;

describe('groupChains', () => {
  it('groups messages in order, one chain per role pair', () => {
    const other = { ...request, id: 3, chainId: 3, to: 'lead' };
    const chains = groupChains([request, other, response], none);

    expect(chains.map((c) => [c.id, c.messages.map((m) => m.id)])).toEqual([
      [1, [1, 2]],
      [3, [3]],
    ]);
  });

  it('is working while any turn runs and nobody answered', () => {
    expect(groupChains([request], { ...none, running: 'tester' })[0].state).toBe('working');
    expect(groupChains([request], { ...none, running: 'developer' })[0].state).toBe('working');
    expect(groupChains([request], { ...none, running: 'lead' })[0].state).toBe('working');
  });

  it('is waiting approval when a pending approval holds a message of the chain', () => {
    expect(groupChains([request], { ...none, approvals: [approval] })[0].state).toBe(
      'waiting approval',
    );
  });

  it('is paused when the channel is paused and nobody answered', () => {
    expect(groupChains([request], { ...none, paused: true })[0].state).toBe('paused');
  });

  it('is failed after a failed response, and stopped when its turn was interrupted', () => {
    const messages = [request, failed];

    expect(groupChains(messages, { ...none, failedTurns: [turn('boom')] })[0].state).toBe('failed');
    expect(groupChains(messages, { ...none, failedTurns: [turn('interrupted')] })[0].state).toBe(
      'stopped',
    );
  });

  it('is working when the recipient runs a retry of a failed attempt', () => {
    const messages = [request, failed];
    const ctx = { ...none, failedTurns: [turn('boom')], running: 'tester' };

    expect(groupChains(messages, ctx)[0].state).toBe('working');
    expect(groupChains(messages, { ...ctx, running: 'lead' })[0].state).toBe('failed');
  });

  it('is rejected or stopped by the body of the failed response of a wake that never ran', () => {
    expect(groupChains([request, { ...failed, body: 'rejected' }], none)[0].state).toBe('rejected');
    expect(groupChains([request, { ...failed, body: 'stopped' }], none)[0].state).toBe('stopped');
  });

  it('is completed after a done response, or when nobody runs and nothing waits', () => {
    expect(groupChains([request, response], none)[0].state).toBe('completed');
    expect(groupChains([request], none)[0].state).toBe('completed');
  });

  it('is one item per role pair and source, counting the messages of every chain', () => {
    const second = { ...request, id: 3, chainId: 3, from: 'tester', to: 'developer' };
    const third = { ...request, id: 4, chainId: 4 };
    const [one] = groupChains([request], none);
    const [two] = groupChains([request, response, second], none);
    const [three] = groupChains([request, response, second, third], none);

    expect([one.messages.length, two.messages.length, three.messages.length]).toEqual([1, 3, 4]);
    expect(three.id).toBe(1);
    expect(groupChains([request, response, second, third], none)).toHaveLength(1);
  });

  it('takes the state of the newest chain of a pair', () => {
    const second = { ...request, id: 3, chainId: 3 };
    const answer = { ...response, id: 4, chainId: 3, replyToId: 3 };

    expect(groupChains([request, failed, second], { ...none, paused: true })[0].state).toBe(
      'paused',
    );
    expect(groupChains([request, failed, second, answer], none)[0].state).toBe('completed');
  });

  it('keeps different pairs, and different sources of one pair, apart', () => {
    const lead = { ...request, id: 3, chainId: 3, to: 'lead' };
    const sourced = { ...request, id: 4, chainId: 4, sourceMessageId: 7 };

    expect(groupChains([request, lead, sourced], none).map((c) => c.id)).toEqual([1, 3, 4]);
  });

  it('keeps the approval and the retryable turn for the panel actions', () => {
    const [waiting] = groupChains([request], { ...none, approvals: [approval] });
    const [broken] = groupChains([request, failed], {
      ...none,
      failedTurns: [turn('boom')],
      retryable: [turn('boom')],
    });

    expect(waiting.approval?.id).toBe(9);
    expect(broken.retryTurn?.id).toBe(5);
  });

  it('stays stopped without a Retry once a later turn made the failed one not retryable', () => {
    const [chain] = groupChains([request, failed], { ...none, failedTurns: [turn('interrupted')] });

    expect(chain.state).toBe('stopped');
    expect(chain.retryTurn).toBeUndefined();
  });
});
