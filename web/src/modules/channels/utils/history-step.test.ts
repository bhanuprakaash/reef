import { describe, expect, it } from 'vitest';
import { stepHistory } from './history-step';

const history = ['newest', 'middle', 'oldest'];

describe('stepHistory', () => {
  it('recalls the newest message only from an empty box', () => {
    expect(stepHistory(history, null, '', 'ArrowUp')).toEqual({ index: 0, text: 'newest' });
    expect(stepHistory(history, null, 'typing', 'ArrowUp')).toBeNull();
    expect(stepHistory([], null, '', 'ArrowUp')).toBeNull();
  });

  it('walks back and forth and clears past the newest', () => {
    expect(stepHistory(history, 0, 'newest', 'ArrowUp')).toEqual({ index: 1, text: 'middle' });
    expect(stepHistory(history, 2, 'oldest', 'ArrowUp')).toEqual({ index: 2, text: 'oldest' });
    expect(stepHistory(history, 1, 'middle', 'ArrowDown')).toEqual({ index: 0, text: 'newest' });
    expect(stepHistory(history, 0, 'newest', 'ArrowDown')).toEqual({ index: null, text: '' });
  });

  it('stops walking once the recalled text is edited', () => {
    expect(stepHistory(history, 1, 'middle!', 'ArrowUp')).toBeNull();
    expect(stepHistory(history, 1, 'middle!', 'ArrowDown')).toBeNull();
  });
});
