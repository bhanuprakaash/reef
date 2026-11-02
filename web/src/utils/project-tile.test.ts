import { describe, expect, it } from 'vitest';
import { projectTile } from './project-tile';

describe('projectTile', () => {
  it('is the first letter on a colour picked by id when nothing is stored', () => {
    expect(projectTile({ id: 2, name: ' shop', icon: '', colour: '' })).toEqual({
      letter: 'S',
      colour: '#6A59E0',
    });
  });

  it('shows the stored emoji and colour', () => {
    expect(projectTile({ id: 2, name: 'shop', icon: '🚀', colour: '#B84E22' })).toEqual({
      letter: '🚀',
      colour: '#B84E22',
    });
  });
});
