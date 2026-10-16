import { describe, expect, it } from 'vitest';
import { addableSkills, isMissing } from './skills';

const available = [
  { name: 'how', description: 'Explain.' },
  { name: 'why', description: 'Motive.' },
  { name: 'slack:standup', description: 'Daily.' },
];

describe('skills', () => {
  it('offers the skills not yet picked', () => {
    expect(addableSkills(available, ['why'])).toEqual(['how', 'slack:standup']);
  });

  it('offers none while the list is unknown', () => {
    expect(addableSkills(undefined, ['why'])).toEqual([]);
  });

  it('calls a picked name missing when it is not on disk', () => {
    expect(isMissing(available, 'how')).toBe(false);
    expect(isMissing(available, 'gone')).toBe(true);
  });

  it('does not call anything missing while the list is unknown', () => {
    expect(isMissing(undefined, 'gone')).toBe(false);
  });
});
