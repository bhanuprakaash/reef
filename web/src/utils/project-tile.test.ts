import { describe, expect, it } from 'vitest';
import { projectTile } from './project-tile';

describe('projectTile', () => {
  it('is the first letter on a colour picked by id when nothing is stored', () => {
    expect(projectTile({ id: 2, name: ' shop', icon: '', colour: '' })).toEqual({
      icon: undefined,
      letter: 'S',
      colour: '#6A59E0',
    });
  });

  it('shows a stored icon name and colour', () => {
    expect(projectTile({ id: 2, name: 'shop', icon: 'rocket', colour: '#B84E22' })).toEqual({
      icon: 'rocket',
      letter: 'S',
      colour: '#B84E22',
    });
  });

  it('falls back to the letter for an icon that is not in the set', () => {
    expect(
      projectTile({ id: 2, name: 'shop', icon: '🚀', colour: '#B84E22' }).icon,
    ).toBeUndefined();
  });
});
