import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter, Route, Routes } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { WorkItemPage } from './work-item';

const hooks = vi.hoisted(() => ({
  useWorkItem: vi.fn(),
  useDiff: vi.fn(),
  none: () => ({ data: [], isPending: false, isError: false }),
}));

vi.mock('@/modules/work-items/api/get-work-item', () => ({ useWorkItem: hooks.useWorkItem }));
vi.mock('@/modules/work-items/api/get-diff', () => ({ useDiff: hooks.useDiff }));
vi.mock('@/modules/work-items/api/get-brief-versions', () => ({ useBriefVersions: hooks.none }));
vi.mock('@/lib/api/get-memory', () => ({ useMemory: hooks.none }));
vi.mock('@/modules/work-items/api/move-work-item', () => ({
  useMoveWorkItem: () => ({ isError: false, mutate: vi.fn() }),
}));
vi.mock('@/modules/projects/api/get-projects', () => ({ useProjects: hooks.none }));
vi.mock('@/lib/api/get-spaces', () => ({ useSpaces: hooks.none }));
vi.mock('@/lib/api/get-teams', () => ({ useTeams: hooks.none }));
vi.mock('@/app/use-terminal', () => ({ useTerminal: () => ({}) }));
vi.mock('@/app/work-item-terminal', () => ({ WorkItemTerminal: () => null }));

function render() {
  return renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={['/work-items/1']}>
        <Routes>
          <Route path="/work-items/:id" element={<WorkItemPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('WorkItemPage header', () => {
  beforeEach(() => {
    hooks.useWorkItem.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        id: 1,
        projectId: 1,
        parentId: null,
        teamId: null,
        acceptanceLines: [],
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

  it('shows the brief, sub-items and proof only on a top-level item', () => {
    hooks.useDiff.mockReturnValue({ isPending: true, isError: false, data: undefined });

    const html = render();

    expect(html).toContain('>Brief<');
    expect(html).toContain('>Sub-items<');
    expect(html).toContain('>Proof<');
  });

  it('links a sub-item to its parent and leaves out the brief, sub-items and proof', () => {
    hooks.useDiff.mockReturnValue({ isPending: true, isError: false, data: undefined });
    hooks.useWorkItem.mockReturnValue({
      isPending: false,
      isError: false,
      data: { ...hooks.useWorkItem().data, id: 2, parentId: 1, title: 'Step', type: 'task' },
    });

    const html = render();

    expect(html).toMatch(/sub-item of <a [^>]*href="\/work-items\/1"/);
    expect(html).not.toContain('>Brief<');
    expect(html).not.toContain('>Sub-items<');
    expect(html).not.toContain('>Proof<');
  });
});
