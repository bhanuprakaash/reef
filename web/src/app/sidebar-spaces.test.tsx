import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { SpacePreview, WorkItem, Project } from '@/types/api';
import { SidebarSpaces } from './sidebar-spaces';

const { useQuery, useWorkItems, useTeams, useInbox, useRunningTurns } = vi.hoisted(() => ({
  useQuery: vi.fn(),
  useWorkItems: vi.fn(),
  useTeams: vi.fn(),
  useInbox: vi.fn(),
  useRunningTurns: vi.fn(),
}));

vi.mock('@tanstack/react-query', async (orig) => {
  const actual = await orig<typeof import('@tanstack/react-query')>();
  return { ...actual, useQuery };
});
vi.mock('@/modules/work-items/api/get-work-items', () => ({
  workItemsQuery: { queryKey: ['work-items'] },
  useWorkItems,
}));
vi.mock('@/lib/api/get-teams', () => ({ useTeams }));
vi.mock('@/modules/inbox/api/get-inbox', () => ({ useInbox }));
vi.mock('@/modules/spaces/api/get-turns', () => ({ useRunningTurns }));

const project: Project = { id: 1, name: 'reef', path: '/reef', baseBranch: 'main', createdAt: '' };

const workItem: WorkItem = {
  id: 2,
  projectId: 1,
  parentId: null,
  type: 'feature',
  title: 'Login',
  description: '',
  status: 'in_progress',
  teamId: null,
  proposedBy: '',
  suggestedTeamId: null,
  queuePosition: null,
  tokenBudget: 0,
  createdAt: '',
  closedAt: null,
  acceptanceLines: [],
};

const space: SpacePreview = {
  id: 4,
  kind: 'work_item',
  projectId: 1,
  workItemId: 2,
  name: 'build',
  team: 'core',
  role: 'lead',
  paused: '',
  createdAt: '',
  closedAt: null,
  movedTo: null,
  lastMessage: null,
};

function render(item: WorkItem = workItem) {
  useQuery.mockImplementation(({ queryKey }: { queryKey: string[] }) => ({
    data: queryKey[0] === 'spaces' ? [space] : [item],
  }));
  useWorkItems.mockReturnValue({ data: [item] });
  useTeams.mockReturnValue({ data: [] });
  useInbox.mockReturnValue({ data: [] });
  return renderToStaticMarkup(
    <MemoryRouter>
      <SidebarSpaces project={project} query="" />
    </MemoryRouter>,
  );
}

describe('SidebarSpaces', () => {
  it('shows a working tooltip on a Space whose bot is running', () => {
    useRunningTurns.mockReturnValue({
      data: [{ spaceId: 4, role: 'developer', status: 'running' }],
    });

    const markup = render();

    expect(markup).toContain('#build');
    expect(markup).toContain('title="developer is working"');
    expect(markup).toContain('animate-ping');
  });

  it('hides the working mark when no turn is running', () => {
    useRunningTurns.mockReturnValue({ data: [] });

    expect(render()).not.toContain('is working');
  });

  it('lists the Space of an in-progress Work item and drops it once the item is in review', () => {
    useRunningTurns.mockReturnValue({ data: [] });

    expect(render()).toContain('#build');
    expect(render({ ...workItem, status: 'review' })).not.toContain('#build');
  });
});
