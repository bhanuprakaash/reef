import { describe, expect, it } from 'vitest';
import { findMention, insertMention } from './mention';

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
