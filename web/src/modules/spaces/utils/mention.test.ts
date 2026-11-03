import { describe, expect, it } from 'vitest';
import { findMention, insertMention, mentionOptions } from './mention';

describe('findMention', () => {
  it('finds the @word before the caret', () => {
    expect(findMention('check @arc', 10)).toEqual({ start: 6, query: 'arc' });
    expect(findMention('@', 1)).toEqual({ start: 0, query: '' });
  });

  it('ignores an @ inside a word or a finished mention', () => {
    expect(findMention('mail me@arc', 11)).toBeNull();
    expect(findMention('@architect plan', 15)).toBeNull();
  });

  it('reads only up to the caret', () => {
    expect(findMention('hi @de later', 6)).toEqual({ start: 3, query: 'de' });
  });
});

describe('insertMention', () => {
  it('replaces the typed word with the role and puts the caret after it', () => {
    expect(insertMention('check @ar', 9, { start: 6, query: 'ar' }, 'architect')).toEqual({
      text: 'check @architect ',
      caret: 17,
    });
  });
});

describe('mentionOptions', () => {
  const members = [
    { botId: 1, name: 'tester' },
    { botId: 2, name: 'architect' },
  ];
  const bots = [
    { id: 1, name: 'tester' },
    { id: 2, name: 'architect' },
    { id: 3, name: 'reviewer' },
    { id: 4, name: 'writer' },
  ];

  it('lists members only for a bare @', () => {
    expect(mentionOptions(members, bots, '')).toEqual([
      { botId: 1, name: 'tester', member: true },
      { botId: 2, name: 'architect', member: true },
    ]);
  });

  it('adds non-members after the members once the prefix matches no member', () => {
    expect(mentionOptions(members, bots, 'r')).toEqual([
      { botId: 3, name: 'reviewer', member: false },
    ]);
    expect(mentionOptions(members, [...bots, { id: 5, name: 'ref' }], 're')).toEqual([
      { botId: 3, name: 'reviewer', member: false },
      { botId: 5, name: 'ref', member: false },
    ]);
  });

  it('drops every non-member once the prefix matches a member', () => {
    expect(mentionOptions(members, bots, 'te')).toEqual([
      { botId: 1, name: 'tester', member: true },
    ]);
  });

  it('matches case-insensitively and lists no one for an unknown prefix', () => {
    expect(mentionOptions(members, bots, 'W')).toEqual([
      { botId: 4, name: 'writer', member: false },
    ]);
    expect(mentionOptions(members, bots, 'zz')).toEqual([]);
  });
});
