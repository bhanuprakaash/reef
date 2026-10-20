import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FirstRun } from './first-run';

const { useCreateProject, useProjects, useProfile } = vi.hoisted(() => ({
  useCreateProject: vi.fn(),
  useProjects: vi.fn(),
  useProfile: vi.fn(),
}));

vi.mock('react-router', () => ({ useNavigate: () => vi.fn() }));
vi.mock('@/lib/api/get-profile', () => ({ useProfile }));
vi.mock('@/lib/api/put-profile', () => ({ usePutProfile: vi.fn() }));
vi.mock('@/modules/projects/api/create-project', () => ({ useCreateProject }));
vi.mock('@/modules/projects/api/get-projects', () => ({ useProjects }));
vi.mock('@/components/creature', () => ({ Creature: () => null }));

function stubWindow(reef?: object) {
  vi.stubGlobal('window', { reef });
}

afterEach(() => vi.unstubAllGlobals());

describe('FirstRun', () => {
  it('shows Choose only when the desktop bridge exists', () => {
    useProfile.mockReturnValue({ data: { name: 'Ada' } });
    useProjects.mockReturnValue({ data: [] });
    useCreateProject.mockReturnValue({ isPending: false, isError: false, mutate: vi.fn() });

    stubWindow();
    const browserMarkup = renderToStaticMarkup(<FirstRun />);
    expect(browserMarkup).not.toContain('Choose…');

    stubWindow({ chooseFolder: vi.fn() });
    const desktopMarkup = renderToStaticMarkup(<FirstRun />);
    expect(desktopMarkup).toContain('Choose…');
  });
});
