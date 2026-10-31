import { describe, expect, it } from 'vitest';
import type { SpacePreview, Message, Bot } from '@/types/api';
import { pickSpace, pickRoot, reviewText } from './send-review';

const space = (id: number, members: number[]): SpacePreview => ({
  id,
  kind: 'work_item',
  projectId: 1,
  workItemId: 1,
  name: `c${id}`,
  team: 'solo',
  role: 'lead',
  botId: 1,
  paused: '',
  createdAt: '',
  closedAt: null,
  movedTo: null,
  members,
  lastMessage: null,
});

const bot = (id: number, name: string): Bot => ({
  id,
  name,
  write: false,
  backend: 'claude',
  permissionMode: '',
  description: '',
  skills: [],
  tools: [],
  mcp: [],
});

const msg = (id: number, parentId: number | null, author: string): Message => ({
  id,
  spaceId: 1,
  parentId,
  author,
  body: `m${id}`,
  createdAt: '',
});

describe('reviewText', () => {
  it('lists each comment as path:line then its text', () => {
    expect(
      reviewText([
        { path: 'a.go', line: 3, text: 'fix typo' },
        { path: 'b.go', line: 10, text: 'add null check' },
      ]),
    ).toBe('a.go:3\nfix typo\n\nb.go:10\nadd null check');
  });

  it('marks a comment on a deleted line as removed', () => {
    expect(reviewText([{ path: 'a.go', line: 3, text: 'was this needed?', removed: true }])).toBe(
      'a.go:3 (removed)\nwas this needed?',
    );
  });
});

describe('pickSpace', () => {
  const bots = [bot(1, 'lead'), bot(2, 'developer')];

  it('picks the most recent Space with a developer among its members, whatever its Crew', () => {
    const spaces = [space(1, [1]), space(2, [1, 2]), space(3, [1, 2])];
    expect(pickSpace(spaces, bots)?.id).toBe(3);
  });

  it('returns undefined when no Space has a developer member', () => {
    expect(pickSpace([space(1, [1])], bots)).toBeUndefined();
  });
});

describe('pickRoot', () => {
  it("returns the id of the developer's latest top-level message", () => {
    const messages = [msg(1, null, 'user'), msg(2, null, 'developer'), msg(3, null, 'developer')];
    expect(pickRoot(messages)).toBe(3);
  });

  it('returns undefined when the developer has no top-level message', () => {
    expect(pickRoot([msg(1, null, 'user'), msg(2, 1, 'developer')])).toBeUndefined();
  });
});
