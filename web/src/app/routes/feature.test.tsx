import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter, Route, Routes } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { FeaturePage } from './feature';

const hooks = vi.hoisted(() => ({
  useFeature: vi.fn(),
  useDiff: vi.fn(),
  none: () => ({ data: [], isPending: false, isError: false }),
}));

vi.mock('@/modules/features/api/get-feature', () => ({ useFeature: hooks.useFeature }));
vi.mock('@/modules/features/api/get-diff', () => ({ useDiff: hooks.useDiff }));
vi.mock('@/modules/features/api/get-brief-versions', () => ({ useBriefVersions: hooks.none }));
vi.mock('@/lib/api/get-memory', () => ({ useMemory: hooks.none }));
vi.mock('@/modules/features/api/set-feature-closed', () => ({
  useSetFeatureClosed: () => ({ isError: false, mutate: vi.fn() }),
}));
vi.mock('@/modules/projects/api/get-projects', () => ({ useProjects: hooks.none }));
vi.mock('@/lib/api/get-spaces', () => ({ useSpaces: hooks.none }));
vi.mock('@/lib/api/get-teams', () => ({ useTeams: hooks.none }));
vi.mock('@/app/use-terminal', () => ({ useTerminal: () => ({}) }));
vi.mock('@/app/feature-terminal', () => ({ FeatureTerminal: () => null }));

function render() {
  return renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/features/1']}>
        <Routes>
          <Route path="/features/:id" element={<FeaturePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('FeaturePage header', () => {
  beforeEach(() => {
    hooks.useFeature.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        id: 1,
        projectId: 1,
        type: 'feature',
        title: 'Auth',
        description: '',
        status: 'backlog',
        closedAt: null,
      },
    });
  });

  it('has no dangling separator while the diff loads', () => {
    hooks.useDiff.mockReturnValue({ isPending: true, isError: false, data: undefined });

    expect(render()).toMatch(/work-item\/1<\/span><\/div>/);
  });

  it('starts the diff summary with its separator once it loads', () => {
    hooks.useDiff.mockReturnValue({ isPending: false, isError: false, data: [] });

    expect(render()).toMatch(/work-item\/1<\/span><span[^>]*>· No changes yet<\/span>/);
  });
});
