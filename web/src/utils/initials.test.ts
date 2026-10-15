import { describe, expect, it } from 'vitest';
import { initials } from './initials';

describe('initials', () => {
  it('takes the first letters of up to two words, uppercase', () => {
    expect(initials('ada')).toBe('A');
    expect(initials('  ada  king lovelace ')).toBe('AK');
    expect(initials('')).toBe('');
  });
});
