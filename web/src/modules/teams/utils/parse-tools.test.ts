import { describe, expect, it } from 'vitest';
import { parseTools } from './parse-tools';

describe('parseTools', () => {
  it('splits one tool per line', () => {
    expect(parseTools('Read\nBash(git log)')).toEqual(['Read', 'Bash(git log)']);
  });

  it('keeps a space inside a tool', () => {
    expect(parseTools('Bash(git log)')).toEqual(['Bash(git log)']);
  });

  it('trims surrounding whitespace and drops blank lines', () => {
    expect(parseTools('  Read  \n\nBash\n')).toEqual(['Read', 'Bash']);
  });

  it('returns an empty list for blank text', () => {
    expect(parseTools('')).toEqual([]);
  });
});
