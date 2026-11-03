import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { SpaceFixes } from './space-fixes';

const { useSpaceWorkspaces, useAbandonSpaceWorkspace } = vi.hoisted(() => ({
  useSpaceWorkspaces: vi.fn(),
  useAbandonSpaceWorkspace: vi.fn(),
}));

vi.mock('@/modules/spaces/api/space-workspaces', () => ({
  useSpaceWorkspaces,
  useAbandonSpaceWorkspace,
}));

const fix = {
  projectId: 1,
  projectName: 'App',
  base: 'develop',
  baseSource: 'local',
  head: 'abc1234',
  dirty: false,
  requests: [],
};

function render(data: unknown[]) {
  useSpaceWorkspaces.mockReturnValue({ isError: false, data });
  useAbandonSpaceWorkspace.mockReturnValue({ isPending: false, isError: false });
  return renderToStaticMarkup(<SpaceFixes spaceId={3} />);
}

describe('SpaceFixes', () => {
  it('lists each open fix with its branch, state and origin note', () => {
    const markup = render([
      {
        ...fix,
        id: 1,
        name: 'login',
        branch: 'fix/3-login',
        state: 'ready',
        note: 'origin could not be refreshed; using local develop at abc1234',
      },
      { ...fix, id: 2, name: 'old', branch: 'fix/3-old', state: 'removed', note: '' },
    ]);

    expect(markup).toContain('login in App');
    expect(markup).toContain('fix/3-login at abc1234');
    expect(markup).toContain('origin could not be refreshed');
    expect(markup).not.toContain('old in App');
  });

  it('links the requests of one fix across repositories in merge order', () => {
    const request = { provider: 'gitlab', state: 'opened', url: 'https://gitlab.com/g/x' };
    const markup = render([
      {
        ...fix,
        id: 1,
        name: 'sync',
        branch: 'fix/3-sync',
        state: 'ready',
        note: '',
        requests: [{ ...request, id: 1, path: 'g/app', number: 12, dependsOn: [2] }],
      },
      {
        ...fix,
        id: 2,
        projectName: 'Lib',
        name: 'sync',
        branch: 'fix/3-sync',
        state: 'ready',
        note: '',
        requests: [{ ...request, id: 2, path: 'g/lib', number: 11, dependsOn: [] }],
      },
    ]);

    expect(markup).toContain('href="https://gitlab.com/g/x"');
    expect(markup).toMatch(/app !12.*merge step 2, after lib !11/);
    expect(markup).toMatch(/lib !11.*merge step 1/);
  });

  it('shows nothing when the conversation has no fix', () => {
    expect(render([])).toBe('');
  });
});
