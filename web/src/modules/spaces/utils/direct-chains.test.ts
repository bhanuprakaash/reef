import { describe, expect, it } from 'vitest';
import type { DirectMessage, Turn } from '@/types/api';
import { groupChains } from './direct-chains';

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
  body: 'Yes',
  status: 'done',
  state: undefined,
};
const answered: DirectMessage = { ...request, state: 'answered' };
const failed: DirectMessage = { ...request, state: 'failed' };
const none = { retryable: [], paused: false };
const turn = (error: string) => ({ id: 5, directMessageId: 1, error }) as Turn;

describe('groupChains', () => {
  it('groups messages in order, one chain per role pair', () => {
    const other = { ...request, id: 3, chainId: 3, to: 'lead', toBotId: 3 };
    const chains = groupChains([request, other, response], none);

    expect(chains.map((c) => [c.id, c.messages.map((m) => m.id)])).toEqual([
      [1, [1, 2]],
      [3, [3]],
    ]);
  });

  it('is working until the request is answered, whatever runs', () => {
    expect(groupChains([request], none)[0].state).toBe('working');
    expect(groupChains([{ ...request, state: 'delivered' }], none)[0].state).toBe('working');
    expect(groupChains([request], { ...none, running: 2 })[0].state).toBe('working');
  });

  it('is paused when the Space is paused and the request is not failed', () => {
    expect(groupChains([request], { ...none, paused: true })[0].state).toBe('paused');
  });

  it('is completed once answered, and paused when its response wake is held by a cap', () => {
    expect(groupChains([answered, response], none)[0].state).toBe('completed');
    expect(groupChains([answered, response], { ...none, paused: true })[0].state).toBe('paused');
  });

  it('never reads completed for a request that failed or was stopped, with a response or not', () => {
    expect(groupChains([failed, response], none)[0].state).toBe('failed');
    expect(groupChains([failed], { ...none, paused: true })[0].state).toBe('failed');
    expect(groupChains([{ ...failed, state: 'stopped' }, response], none)[0].state).toBe('stopped');
  });

  it('is working when the recipient runs a retry of a failed attempt', () => {
    expect(groupChains([failed], { ...none, running: 2 })[0].state).toBe('working');
    expect(groupChains([failed], { ...none, running: 3 })[0].state).toBe('failed');
  });

  it('is one item per role pair and source, counting the messages of every chain', () => {
    const second = {
      ...request,
      id: 3,
      chainId: 3,
      from: 'tester',
      fromBotId: 2,
      to: 'developer',
      toBotId: 1,
    };
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
    const secondAnswered = { ...second, state: 'answered' } as const;

    expect(groupChains([failed, second], { ...none, paused: true })[0].state).toBe('paused');
    expect(groupChains([failed, secondAnswered, answer], none)[0].state).toBe('completed');
  });

  it('keeps different pairs, and different sources of one pair, apart', () => {
    const lead = { ...request, id: 3, chainId: 3, to: 'lead', toBotId: 3 };
    const sourced = { ...request, id: 4, chainId: 4, sourceMessageId: 7 };

    expect(groupChains([request, lead, sourced], none).map((c) => c.id)).toEqual([1, 3, 4]);
  });

  it('keeps the retryable turn for the panel actions', () => {
    const [broken] = groupChains([failed], { ...none, retryable: [turn('boom')] });

    expect(broken.retryTurn?.id).toBe(5);
  });

  it('stays stopped without a Retry once a later turn made the stopped one not retryable', () => {
    const [chain] = groupChains([{ ...failed, state: 'stopped' }], none);

    expect(chain.state).toBe('stopped');
    expect(chain.retryTurn).toBeUndefined();
  });
});
