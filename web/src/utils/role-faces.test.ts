import { describe, expect, it } from 'vitest';
import { faceOf } from '@/components/creature';
import type { Bot } from '@/types/api';
import { memberFaces } from './role-faces';

const bots = [
  { id: 7, name: 'sally', face: 'whale' },
  { id: 8, name: 'bo', face: 'crab' },
] as Bot[];

describe('memberFaces', () => {
  it('keeps the face of a renamed Bot, found by its id', () => {
    expect(faceOf(memberFaces(bots, [7]), 7)?.face).toBe('whale');
  });

  it('gives a Bot outside the Space no face', () => {
    expect(faceOf(memberFaces(bots, [7]), 8)).toBeUndefined();
  });

  it('has no face for a Space with no Bot id', () => {
    expect(faceOf(memberFaces(bots, [7]), null)).toBeUndefined();
  });
});
