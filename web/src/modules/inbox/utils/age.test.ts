import { expect, it } from 'vitest';
import { age } from './age';

it('shows the largest whole unit', () => {
  const now = Date.parse('2026-10-14T12:00:00Z');
  expect(age('2026-10-14T11:59:40Z', now)).toBe('now');
  expect(age('2026-10-14T11:58:00Z', now)).toBe('2m');
  expect(age('2026-10-14T10:30:00Z', now)).toBe('1h');
  expect(age('2026-10-11T12:00:00Z', now)).toBe('3d');
});
