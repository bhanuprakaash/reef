import { describe, expect, it } from 'vitest';
import { suggestFace } from '@/modules/teams/utils/suggest-face';

describe('suggestFace', () => {
  it('suggests a creature from the role name', () => {
    expect(
      ['Researcher', 'security', 'designer', 'writer', 'product', 'devops'].map(suggestFace),
    ).toEqual(['octopus', 'turtle', 'seahorse', 'otter', 'clownfish', 'manta-ray']);
  });

  it('falls back to the role default', () => {
    expect(['tester', 'qa', ''].map(suggestFace)).toEqual(['shark', 'pebble', 'pebble']);
  });
});
