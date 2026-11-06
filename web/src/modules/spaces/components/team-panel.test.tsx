import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { Role } from '@/types/api';
import { BotProfile, TeamPanel } from './team-panel';

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

function profile(r: Role, ...others: Role[]) {
  return renderToStaticMarkup(
    <MemoryRouter>
      <BotProfile
        spaceId={1}
        role={r}
        roles={[r, ...others]}
        state="idle"
        tokens={12400}
        onBack={() => {}}
        onClose={() => {}}
      />
    </MemoryRouter>,
  ).replace(/<[^>]*>/g, ' ');
}

describe('TeamPanel', () => {
  it('lists each Bot with its session backend and state, and shows no profile until one is opened', () => {
    const markup = render(role);

    expect(markup).toContain('aria-label="Open prakash"');
    expect(markup).toContain('Claude Code · idle');
    expect(markup).not.toContain('Apply here');
  });
});

describe('BotProfile', () => {
  it('shows what the session runs with, not the Bot record', () => {
    const text = profile(role).replace(/\s+/g, ' ');

    expect(text).toContain('Model Sonnet');
    expect(text).toContain('Permissions acceptEdits · read-only');
    expect(text).toContain('Backend Claude Code');
    expect(text).toContain('Usage 12.4k tokens in this Space');
    expect(text).toContain('Tools 1 Read');
    expect(text).toContain('Skills 2 claude:how codex:lint');
    expect(text).toContain('Connections 1 files');
    expect(text).toContain('codex:lint is not available here: needs Codex');
    expect(text).toContain('Bot settings changed. Apply here to adopt edits.');
  });

  it('collapses a long tool list to the first three and a count', () => {
    const tools = ['Read', 'Grep', 'Glob', 'Skill', 'Bash(git log:*)'];
    const text = profile({ ...role, session: { ...role.session!, tools } }).replace(/\s+/g, ' ');

    expect(text).toContain('Tools 5 Read Grep Glob +2');
    expect(text).not.toContain('Bash(git log:*)');
  });

  it('offers Remove, with a successor to pick for the coordinator, only when another Bot stays', () => {
    expect(profile(role)).not.toContain('Remove');

    const mira = { ...role, botId: 2, name: 'mira', lead: false };
    const lead = profile(role, mira);

    expect(lead).toContain('Coordinator after prakash leaves');
    expect(lead).toContain('Remove');
    expect(lead).not.toContain('Make coordinator');

    const other = profile(mira, role);

    expect(other).not.toContain('Coordinator after mira leaves');
    expect(other).toContain('Make coordinator');
    expect(other).toContain('Edit Bot');
  });

  it('shows a pending removal from the Space data, not from the click', () => {
    const text = profile({ ...role, botId: 2, name: 'mira', lead: false, leaving: true }, role);

    expect(text).toContain('Leaves after its current turn.');
    expect(text).not.toContain('Remove');
  });

  it('shows no hint when the session matches the Bot', () => {
    expect(profile({ ...role, session: { ...role.session!, stale: false } })).not.toContain(
      'to adopt edits',
    );
  });
});
