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
vi.mock('@/modules/spaces/api/remove-member', () => ({
  useRemoveMember: () => ({ isPending: false, isError: false, mutate: () => {}, data: undefined }),
}));
vi.mock('@/modules/spaces/components/space-projects', () => ({ SpaceProjects: () => null }));
vi.mock('@/modules/spaces/components/space-fixes', () => ({ SpaceFixes: () => null }));

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

function render(r: Role, ...others: Role[]) {
  return renderToStaticMarkup(
    <MemoryRouter>
      <TeamPanel
        spaceId={1}
        workItemId={null}
        roles={[r, ...others]}
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
    expect(markup).toContain('aria-label="Model for prakash"');
    expect(markup).toContain('<span class="truncate">Sonnet</span>');
    expect(markup).toContain('<dt>Model</dt><dd class="min-w-0 break-words text-soft">sonnet</dd>');
    expect(markup).toContain('acceptEdits · read-only');
    expect(markup).toContain('<dt>Tools</dt><dd class="min-w-0 break-words text-soft">Read</dd>');
    expect(markup).toContain('claude:how, codex:lint');
    expect(markup).toContain('codex:lint is not available here: needs Codex');
    expect(markup).toContain('Bot settings changed. Apply to open chats to adopt edits.');
  });

  it('collapses a long tool list to a count', () => {
    const tools = ['Read', 'Grep', 'Glob', 'Skill', 'Bash(git log:*)'];
    const markup = render({ ...role, session: { ...role.session!, tools } });

    expect(markup).toContain('>5 tools</button>');
    expect(markup).not.toContain('Read, Grep');
  });

  it('offers Remove, with a successor to pick for the coordinator, only when another Bot stays', () => {
    expect(render(role)).not.toContain('Remove');

    const markup = render(role, { ...role, botId: 2, name: 'mira', lead: false });

    expect(markup).toContain('aria-label="Coordinator after prakash leaves"');
    expect(markup).not.toContain('Coordinator after mira leaves');
    expect(markup.match(/>Remove</g)).toHaveLength(2);
  });

  it('shows a pending removal from the Space data, not from the click', () => {
    const markup = render(role, { ...role, botId: 2, name: 'mira', lead: false, leaving: true });

    expect(markup).toContain('Leaves after its current turn.');
    expect(markup.match(/>Remove</g)).toHaveLength(1);
  });

  it('shows no hint when the session matches the Bot', () => {
    const markup = render({ ...role, session: { ...role.session!, stale: false } });

    expect(markup).not.toContain('to adopt edits');
  });
});
