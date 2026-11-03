import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, it, vi } from 'vitest';
import type { Project } from '@/types/api';
import { ProjectSwitcher } from './project-switcher';

const projects: Project[] = [
  {
    id: 1,
    name: 'reef',
    path: '/reef',
    purpose: '',
    baseBranch: 'main',
    icon: '',
    colour: '',
    createdAt: '',
    repoConnectionId: null,
    remotePath: '',
  },
  {
    id: 2,
    name: 'site',
    path: '/site',
    purpose: '',
    baseBranch: 'main',
    icon: '',
    colour: '',
    createdAt: '',
    repoConnectionId: null,
    remotePath: '',
  },
];

function render(stored: string | null) {
  vi.stubGlobal('localStorage', { getItem: () => stored, setItem: () => {} });
  return renderToStaticMarkup(
    <MemoryRouter>
      <ProjectSwitcher projects={projects} />
    </MemoryRouter>,
  );
}

afterEach(() => vi.unstubAllGlobals());

it('shows All projects when that was the last pick', () => {
  expect(render('all')).toContain('All projects');
});

it('shows the stored project, or the first one when nothing is stored', () => {
  expect(render('2')).toContain('site');
  expect(render(null)).toContain('reef');
});

it('shows the project initial in its tile', () => {
  expect(render('2')).toContain('>S<');
});
