import { describe, expect, it } from 'vitest';
import { creatureName } from './creature';

describe('creatureName', () => {
  it('maps the four built-in roles', () => {
    expect(['lead', 'architect', 'developer', 'tester'].map((r) => creatureName(r))).toEqual([
      'jellyfish',
      'whale',
      'penguin',
      'shark',
    ]);
  });

  it('resolves a library creature by face, over the role', () => {
    expect(creatureName('researcher', 'octopus')).toBe('octopus');
    expect(creatureName('lead', 'manta-ray')).toBe('manta-ray');
  });

  it('falls back to a pebble for an unknown role, an unknown face or a library name used as a role', () => {
    expect(creatureName('security')).toBe('pebble');
    expect(creatureName('security', 'dragon')).toBe('pebble');
    expect(creatureName('turtle')).toBe('pebble');
    expect(creatureName('constructor', 'toString')).toBe('pebble');
  });
});
