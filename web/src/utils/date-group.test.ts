import { describe, expect, it } from 'vitest';
import { dateGroup } from './date-group';

const now = new Date(2026, 9, 15, 12, 0);
const at = (month: number, day: number, hour: number, minute = 0) =>
  new Date(2026, month, day, hour, minute).toISOString();

describe('dateGroup', () => {
  it('buckets by calendar day', () => {
    expect(dateGroup(at(9, 15, 0, 1), now)).toBe('Today');
    expect(dateGroup(at(9, 14, 23, 59), now)).toBe('Yesterday');
    expect(dateGroup(at(9, 14, 0, 0), now)).toBe('Yesterday');
    expect(dateGroup(at(9, 13, 23, 59), now)).toBe('Last 7 days');
    expect(dateGroup(at(9, 9, 0, 0), now)).toBe('Last 7 days');
    expect(dateGroup(at(9, 8, 23, 59), now)).toBe('Older');
    expect(dateGroup(at(8, 1, 10), now)).toBe('Older');
  });
});
