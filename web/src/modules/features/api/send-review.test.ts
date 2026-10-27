import { describe, expect, it } from 'vitest';
import type { Space, Message, Team } from '@/types/api';
import { pickSpace, pickRoot, reviewText } from './send-review';

const space = (id: number, team: string): Space => ({
  id,
  kind: 'work_item',
  projectId: 1,
  workItemId: 1,
  name: `c${id}`,
  team,
  role: 'lead',
  backend: 'claude',
  permissionMode: 'acceptEdits',
  paused: '',
  createdAt: '',
  closedAt: null,
});

const team = (name: string, roles: string[]): Team => ({
  name,
  roles: roles.map((r) => ({
    name: r,
    lead: r === 'lead',
    write: false,
    backend: 'claude',
    permissionMode: '',
    persona: '',
    skills: [],
    tools: [],
    mcp: [],
  })),
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
  it('picks the most recent Space whose team has a developer role', () => {
    const teams = [team('solo', ['lead']), team('full', ['lead', 'developer'])];
    const spaces = [space(1, 'solo'), space(2, 'full'), space(3, 'full')];
    expect(pickSpace(spaces, teams)?.id).toBe(3);
  });

  it('returns undefined when no Space has a developer role', () => {
    expect(pickSpace([space(1, 'solo')], [team('solo', ['lead'])])).toBeUndefined();
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
