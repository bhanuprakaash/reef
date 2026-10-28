import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { Creature, creatureName } from './creature';

describe('creatureName', () => {
  it('resolves a creature by its face', () => {
    expect(['jellyfish', 'whale', 'penguin', 'shark', 'manta-ray'].map(creatureName)).toEqual([
      'jellyfish',
      'whale',
      'penguin',
      'shark',
      'manta-ray',
    ]);
  });

  it('falls back to a pebble when the face is empty or unknown', () => {
    expect([undefined, '', 'dragon', 'constructor', 'toString'].map(creatureName)).toEqual([
      'pebble',
      'pebble',
      'pebble',
      'pebble',
      'pebble',
    ]);
  });
});

describe('Creature', () => {
  it('paints the body in its colour, or in its own colour without one', () => {
    const paint = (colour?: string) =>
      renderToStaticMarkup(<Creature look={{ face: 'jellyfish', colour }} size={20} />);

    expect(paint('violet')).toContain('fill="#8B5CF6"');
    expect(paint('violet')).not.toContain('#F26B2E');
    expect(paint()).toContain('fill="#F26B2E"');
  });
});
