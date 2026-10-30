import { describe, expect, it } from 'vitest';
import { faceOf } from '@/components/creature';
import type { Team } from '@/types/api';
import { roleFaces } from './role-faces';

const teams = [
  { id: 1, name: 'core', roles: [{ botId: 7, name: 'sally', face: 'whale' }] },
] as Team[];

describe('roleFaces', () => {
  it('keeps the face of a renamed Bot, found by its id', () => {
    expect(faceOf(roleFaces(teams, 'core'), 7)?.face).toBe('whale');
  });

  it('gives a different Bot that reuses the old name no face', () => {
    expect(faceOf(roleFaces(teams, 'core'), 9)).toBeUndefined();
  });

  it('has no face for a Space with no Bot id', () => {
    expect(faceOf(roleFaces(teams, 'core'), null)).toBeUndefined();
  });
});
