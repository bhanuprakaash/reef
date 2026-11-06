import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';
import type { Role, SpaceDetail } from '@/types/api';
import { SpaceHeader } from './space-header';

const space: SpaceDetail = {
  id: 1,
  kind: 'free',
  projectId: 1,
  workItemId: null,
  name: 'login flow',
  team: '',
  role: '',
  botId: null,
  paused: '',
  createdAt: '',
  closedAt: null,
  movedTo: null,
  tokenBudget: 1000,
  tokensUsed: 10,
};

const bot = { botId: 3, name: 'sam', lead: true } as Role;

function render(over: Partial<SpaceDetail>) {
  return renderToStaticMarkup(
    <MemoryRouter>
      <SpaceHeader
        space={{ ...space, ...over }}
        scope="free Space"
        roles={[bot]}
        faces={{}}
        working={null}
        pullRequest={null}
        onPullRequest={() => {}}
        pullRequestOpen={false}
        wrapDisabled={false}
        onWrap={() => {}}
        attachDisabled={false}
        onAttach={() => {}}
        closeDisabled={false}
        onClose={() => {}}
        onAddBot={() => {}}
        onNewChat={() => {}}
        newChatDisabled={false}
        onInspector={() => {}}
        onTeam={() => {}}
      />
    </MemoryRouter>,
  );
}

describe('SpaceHeader', () => {
  it('offers New chat in a conversation only', () => {
    expect(render({ kind: 'conversation' })).toContain('aria-label="New chat"');
    expect(render({ kind: 'free' })).not.toContain('New chat');
    expect(render({ kind: 'work_item', workItemId: 4 })).not.toContain('New chat');
  });

  it('offers Wrap up on an open Work item Space only', () => {
    const open = { kind: 'work_item', workItemId: 4 } as const;
    expect(render(open)).toContain('>Wrap up<');
    expect(render({ ...open, closedAt: '2026-01-01T00:00:00Z' })).not.toContain('Wrap up');
    expect(render({ kind: 'free' })).not.toContain('Wrap up');
  });
});
