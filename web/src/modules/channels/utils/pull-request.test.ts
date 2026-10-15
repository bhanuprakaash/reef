import { describe, expect, it } from 'vitest';
import type { Message } from '@/types/api';
import { latestPullRequest } from './pull-request';

const note = (id: number, author: string, body: string): Message => ({
  id,
  channelId: 1,
  parentId: null,
  author,
  body,
  createdAt: '2026-10-15T10:00:00Z',
});

describe('latestPullRequest', () => {
  it('takes the newest PR note from reef', () => {
    const messages = [
      note(1, 'reef', 'PR #7 open: https://github.com/o/r/pull/7'),
      note(2, 'reef', 'lint failed on PR #7: https://github.com/o/r/pull/7'),
      note(3, 'reef', 'PR #7 merged: https://github.com/o/r/pull/7'),
    ];
    expect(latestPullRequest(messages)).toEqual({
      number: 7,
      state: 'merged',
      url: 'https://github.com/o/r/pull/7',
    });
  });

  it('ignores a user message that looks like a note', () => {
    expect(latestPullRequest([note(1, 'user', 'PR #7 open: https://x.test/7')])).toBeNull();
  });
});
