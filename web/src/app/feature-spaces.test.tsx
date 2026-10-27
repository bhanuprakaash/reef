import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { SpacePreview, WorkItem, Project } from '@/types/api';
import { FeatureSpaces } from './feature-spaces';

const { useQuery, useFeatures, useTeams, useInbox, useRunningTurns } = vi.hoisted(() => ({
  useQuery: vi.fn(),
  useFeatures: vi.fn(),
  useTeams: vi.fn(),
  useInbox: vi.fn(),
  useRunningTurns: vi.fn(),
}));

vi.mock('@tanstack/react-query', async (orig) => {
  const actual = await orig<typeof import('@tanstack/react-query')>();
  return { ...actual, useQuery };
});
vi.mock('@/modules/features/api/get-features', () => ({ useFeatures }));
vi.mock('@/lib/api/get-teams', () => ({ useTeams }));
vi.mock('@/modules/inbox/api/get-inbox', () => ({ useInbox }));
vi.mock('@/modules/spaces/api/get-turns', () => ({ useRunningTurns }));

const project: Project = { id: 1, name: 'reef', path: '/reef', baseBranch: 'main', createdAt: '' };

const feature: WorkItem = {
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
  backend: 'claude',
  permissionMode: 'default',
  paused: '',
  createdAt: '',
  closedAt: null,
  movedTo: null,
  lastMessage: null,
};

function render() {
  return renderToStaticMarkup(
    <MemoryRouter>
      <FeatureSpaces project={project} query="" />
    </MemoryRouter>,
  );
}

describe('FeatureSpaces', () => {
  it('shows a working tooltip on a Space whose teammate is running', () => {
    useQuery.mockReturnValue({ data: [space] });
    useFeatures.mockReturnValue({ data: [feature] });
    useTeams.mockReturnValue({ data: [] });
    useInbox.mockReturnValue({ data: [] });
    useRunningTurns.mockReturnValue({
      data: [{ spaceId: 4, role: 'developer', status: 'running' }],
    });

    const markup = render();

    expect(markup).toContain('#build');
    expect(markup).toContain('title="developer is working"');
    expect(markup).toContain('animate-ping');
  });

  it('hides the working mark when no turn is running', () => {
    useQuery.mockReturnValue({ data: [space] });
    useFeatures.mockReturnValue({ data: [feature] });
    useTeams.mockReturnValue({ data: [] });
    useInbox.mockReturnValue({ data: [] });
    useRunningTurns.mockReturnValue({ data: [] });

    expect(render()).not.toContain('is working');
  });
});
