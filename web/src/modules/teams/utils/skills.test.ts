import { describe, expect, it } from 'vitest';
import { addableSkills, isMissing } from './skills';

const available = [
  { ref: 'reef:how', name: 'how', description: 'Library.', source: 'reef' as const, version: 1 },
  { ref: 'claude:how', name: 'how', description: 'Explain.', source: 'claude' as const },
  { ref: 'codex:why', name: 'why', description: 'Motive.', source: 'codex' as const },
];

describe('skills', () => {
  it('offers the skills not yet picked, keeping equal names from other sources', () => {
    expect(addableSkills(available, ['claude:how']).map((s) => s.ref)).toEqual([
      'reef:how',
      'codex:why',
    ]);
  });

  it('offers none while the list is unknown', () => {
    expect(addableSkills(undefined, ['why'])).toEqual([]);
  });

  it('calls a picked ref missing when it is not found', () => {
    expect(isMissing(available, 'reef:how')).toBe(false);
    expect(isMissing(available, 'claude:gone')).toBe(true);
  });

  it('does not call anything missing while the list is unknown', () => {
    expect(isMissing(undefined, 'gone')).toBe(false);
  });
});
