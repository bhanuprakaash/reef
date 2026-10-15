import { describe, expect, it } from 'vitest';
import { showTimestamp } from './message-time';

describe('showTimestamp', () => {
  it('shows one before the first message', () => {
    expect(showTimestamp(undefined, '2026-10-15T10:00:00Z')).toBe(true);
  });

  it('stays hidden at exactly ten minutes', () => {
    expect(showTimestamp('2026-10-15T10:00:00Z', '2026-10-15T10:10:00Z')).toBe(false);
  });

  it('shows one when the gap is over ten minutes', () => {
    expect(showTimestamp('2026-10-15T10:00:00Z', '2026-10-15T10:10:01Z')).toBe(true);
  });

  it('stays hidden for messages seconds apart', () => {
    expect(showTimestamp('2026-10-15T10:00:00Z', '2026-10-15T10:00:30Z')).toBe(false);
  });
});
