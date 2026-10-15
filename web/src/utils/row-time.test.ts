import { describe, expect, it } from 'vitest';
import { rowTime } from './row-time';

const now = new Date(2026, 9, 15, 12, 0);

describe('rowTime', () => {
  it('shows the clock for today, the weekday for this week and the date after that', () => {
    expect(rowTime(new Date(2026, 9, 15, 9, 12).toISOString(), now)).toBe('09:12');
    expect(rowTime(new Date(2026, 9, 12, 20, 0).toISOString(), now)).toBe('Mon');
    expect(rowTime(new Date(2026, 8, 20, 20, 0).toISOString(), now)).toBe('Sep 20');
  });
});
