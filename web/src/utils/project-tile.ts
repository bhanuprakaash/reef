import { projectIconNames } from '@/components/project-icons';
import type { Project } from '@/types/api';

export const palette = [
  '#B84E22',
  '#1F7A6F',
  '#6A59E0',
  '#B03E73',
  '#2F72C0',
  '#5F7A1E',
  '#8F6A12',
  '#5E6B78',
];

export const projectTile = (p: Pick<Project, 'id' | 'name' | 'icon' | 'colour'>) => ({
  icon: projectIconNames.includes(p.icon) ? p.icon : undefined,
  letter: p.name.trim()[0]?.toUpperCase() ?? '?',
  colour: p.colour || palette[p.id % palette.length],
});
