import { describe, expect, it } from 'vitest';
import { blocksUser } from './blocks-user';

describe('blocksUser', () => {
  it('does not block when the channel is not paused', () => {
    expect(blocksUser('')).toBe(false);
  });

  it('does not block on the hop cap pause', () => {
    expect(blocksUser('hop cap: 4 agent hops without a user message')).toBe(false);
  });

  it('blocks on other pauses', () => {
    expect(blocksUser('token budget used: 1000 of 1000')).toBe(true);
    expect(blocksUser('tool cap reached')).toBe(true);
  });
});
