import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { Role } from '@/types/api';
import { TeamPanel } from './team-panel';

vi.mock('@/modules/spaces/api/get-inspector', () => ({
  useInspector: () => ({ data: undefined }),
}));
vi.mock('@/modules/spaces/api/set-coordinator', () => ({
  useSetCoordinator: () => ({ isPending: false, isError: false, mutate: () => {} }),
}));
vi.mock('@/modules/spaces/api/refresh-bot', () => ({
  useRefreshBot: () => ({ isPending: false, isError: false, isSuccess: false, mutate: () => {} }),
}));
vi.mock('@/modules/spaces/components/space-projects', () => ({ SpaceProjects: () => null }));

const role: Role = {
  botId: 1,
  name: 'prakash',
  lead: true,
  write: true,
  backend: 'codex',
  model: 'opus',
  permissionMode: 'acceptEdits',
  description: '',
  skills: [],
  tools: [],
  connections: [],
  session: {
    backend: 'claude',
    model: 'sonnet',
    permissionMode: 'acceptEdits',
    write: false,
    tools: ['Read'],
    skills: [
      { ref: 'claude:how', compatible: true },
      { ref: 'codex:lint', compatible: false, reason: 'needs Codex' },
    ],
    connections: ['files'],
    refreshPending: false,
    stale: true,
  },
};

function render(r: Role) {
  return renderToStaticMarkup(
    <MemoryRouter>
      <TeamPanel
        spaceId={1}
        workItemId={null}
        roles={[r]}
        projects={[]}
        working={null}
        onClose={() => {}}
      />
    </MemoryRouter>,
  );
}

describe('TeamPanel', () => {
  it('shows what the session runs with, not the Bot record, and the refresh hint', () => {
    const markup = render(role);

    expect(markup).toContain('Claude Code');
    expect(markup).toContain('placeholder="sonnet"');
    expect(markup).toContain(
      'Session runs with sonnet · acceptEdits · read-only · tools: Read · skills: claude:how, codex:lint · connections: files',
    );
    expect(markup).toContain('codex:lint is not available here: needs Codex');
    expect(markup).toContain('Bot settings changed. Refresh to adopt edits.');
  });

  it('shows no hint when the session matches the Bot', () => {
    const markup = render({ ...role, session: { ...role.session!, stale: false } });

    expect(markup).not.toContain('Refresh to adopt edits');
  });
});
