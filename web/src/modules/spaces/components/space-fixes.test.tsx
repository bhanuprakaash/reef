import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { SpaceFixes } from './space-fixes';

const { useSpaceWorkspaces, useAbandonSpaceWorkspace } = vi.hoisted(() => ({
  useSpaceWorkspaces: vi.fn(),
  useAbandonSpaceWorkspace: vi.fn(),
}));

vi.mock('@/modules/spaces/components/request-list', () => ({ RequestList: () => null }));

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

  it('shows nothing when the conversation has no fix', () => {
    expect(render([])).toBe('');
  });
});
