import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { ChannelPreview, Feature, Project } from '@/types/api';
import { FeatureChannels } from './feature-channels';

const { useQuery, useFeatures, useTeams, useInbox, useTasks, useRunningTurns } = vi.hoisted(() => ({
  useQuery: vi.fn(),
  useFeatures: vi.fn(),
  useTeams: vi.fn(),
  useInbox: vi.fn(),
  useTasks: vi.fn(),
  useRunningTurns: vi.fn(),
}));

vi.mock('@tanstack/react-query', async (orig) => {
  const actual = await orig<typeof import('@tanstack/react-query')>();
  return { ...actual, useQuery };
});
vi.mock('@/modules/features/api/get-features', () => ({ useFeatures }));
vi.mock('@/lib/api/get-teams', () => ({ useTeams }));
vi.mock('@/modules/inbox/api/get-inbox', () => ({ useInbox }));
vi.mock('@/modules/tasks/api/get-tasks', () => ({ useTasks }));
vi.mock('@/modules/channels/api/get-turns', () => ({ useRunningTurns }));

const project: Project = { id: 1, name: 'reef', path: '/reef', baseBranch: 'main', createdAt: '' };

const feature: Feature = {
  id: 2,
  projectId: 1,
  slug: 'login',
  name: 'Login',
  goal: '',
  stage: 'implement',
  kind: 'journey',
  createdAt: '',
  closedAt: null,
};

const channel: ChannelPreview = {
  id: 4,
  projectId: 1,
  featureId: 2,
  name: 'build',
  team: 'core',
  role: 'lead',
  backend: 'claude',
  permissionMode: 'default',
  paused: '',
  createdAt: '',
  closedAt: null,
  lastMessage: null,
};

function render() {
  return renderToStaticMarkup(
    <MemoryRouter>
      <FeatureChannels project={project} query="" />
    </MemoryRouter>,
  );
}

describe('FeatureChannels', () => {
  it('shows a working tooltip on a channel whose teammate is running', () => {
    useQuery.mockReturnValue({ data: [channel] });
    useFeatures.mockReturnValue({ data: [feature] });
    useTeams.mockReturnValue({ data: [] });
    useInbox.mockReturnValue({ data: [] });
    useTasks.mockReturnValue({ data: [] });
    useRunningTurns.mockReturnValue({
      data: [{ channelId: 4, role: 'developer', status: 'running' }],
    });

    const markup = render();

    expect(markup).toContain('#build');
    expect(markup).toContain('title="developer is working"');
    expect(markup).toContain('animate-ping');
  });

  it('hides the working mark when no turn is running', () => {
    useQuery.mockReturnValue({ data: [channel] });
    useFeatures.mockReturnValue({ data: [feature] });
    useTeams.mockReturnValue({ data: [] });
    useInbox.mockReturnValue({ data: [] });
    useTasks.mockReturnValue({ data: [] });
    useRunningTurns.mockReturnValue({ data: [] });

    expect(render()).not.toContain('is working');
  });
});
