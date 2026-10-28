import { describe, expect, it } from 'vitest';
import { messageTime, showTimestamp } from './message-time';

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

describe('messageTime', () => {
  const now = new Date('2026-10-15T12:00:00Z');
  const clock = /^\d{1,2}:\d{2}\s?[ap]m?$/i;

  it('shows only the clock time for today', () => {
    expect(messageTime('2026-10-15T10:30:00Z', now)).toMatch(clock);
  });

  it('prefixes the day for a message from another day', () => {
    const label = messageTime('2026-10-14T10:30:00Z', now);
    expect(label).toContain('Oct');
    expect(label).toMatch(/\d{1,2}:\d{2}/);
  });
});
