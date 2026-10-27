import { describe, expect, it } from 'vitest';
import { formatTokens } from './format-tokens';

describe('formatTokens', () => {
  it('shows small counts as they are', () => {
    expect(formatTokens(999)).toBe('999');
  });

  it('shows thousands with one decimal', () => {
    expect(formatTokens(312400)).toBe('312.4k');
  });
});
