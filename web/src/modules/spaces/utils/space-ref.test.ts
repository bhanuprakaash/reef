import { describe, expect, it } from 'vitest';
import { findSpaceRef, insertSpaceRef } from './space-ref';

describe('findSpaceRef', () => {
  it('finds the #word before the caret', () => {
    expect(findSpaceRef('why in #log', 11)).toEqual({ start: 7, query: 'log' });
  });

  it('ignores a # inside a word or a finished reference', () => {
    expect(findSpaceRef('issue#12', 8)).toBeNull();
    expect(findSpaceRef('#[Login](space:3) ok', 20)).toBeNull();
  });
});

describe('insertSpaceRef', () => {
  it('writes the id token and puts the caret after it', () => {
    expect(
      insertSpaceRef('why in #lo', 10, { start: 7, query: 'lo' }, { id: 3, name: 'Login' }),
    ).toEqual({ text: 'why in #[Login](space:3) ', caret: 25 });
  });
});
