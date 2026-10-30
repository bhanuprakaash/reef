import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { SpacePreview, WorkItem, FileDiff } from '@/types/api';
import { SpaceCard, spaceState } from './space-hovercard';

const { useDiff } = vi.hoisted(() => ({ useDiff: vi.fn() }));
vi.mock('@/modules/work-items/api/get-diff', () => ({ useDiff }));

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
  team: 'full',
  role: 'lead',
  botId: 1,
  paused: '',
  createdAt: '',
  closedAt: null,
  movedTo: null,
  lastMessage: null,
};

const files: FileDiff[] = [
  {
    path: 'a.txt',
    status: 'modified',
    hunks: [
      {
        header: '',
        lines: [
          { kind: 'add', text: 'x' },
          { kind: 'add', text: 'y' },
          { kind: 'del', text: 'z' },
          { kind: 'ctx', text: 'c' },
        ],
      },
    ],
  },
  { path: 'b.txt', status: 'added', hunks: [{ header: '', lines: [{ kind: 'add', text: 'q' }] }] },
];

const members = { 1: { name: 'lead' }, 2: { name: 'developer', face: 'penguin' } };
const text = (markup: string) => markup.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ');

describe('SpaceCard', () => {
  it('shows the branch, the changes and who is working', () => {
    useDiff.mockReturnValue({ data: files });
    const markup = renderToStaticMarkup(
      <SpaceCard space={space} workItem={workItem} members={members} working={[2]} waiting={[]} />,
    );
    expect(text(markup)).toBe(
      ' #build Working Branch work-item/2 Changes 2 files +3 −1 lead idle developer working ',
    );
    expect(useDiff).toHaveBeenCalledWith(2);
  });

  it('has no branch or changes for a free Space and does not fetch a diff', () => {
    useDiff.mockClear();
    const markup = renderToStaticMarkup(
      <SpaceCard
        space={{ ...space, workItemId: null }}
        workItem={undefined}
        members={members}
        working={[]}
        waiting={[]}
      />,
    );
    expect(text(markup)).toBe(' #build Idle lead idle developer idle ');
    expect(useDiff).not.toHaveBeenCalled();
  });

  it('marks working by Bot id, so a renamed Bot and a new Bot with its old name stay apart', () => {
    const renamed = { 7: { name: 'sally' }, 9: { name: 'sam' } };
    const markup = renderToStaticMarkup(
      <SpaceCard
        space={{ ...space, workItemId: null }}
        workItem={undefined}
        members={renamed}
        working={[7]}
        waiting={[]}
      />,
    );
    expect(text(markup)).toBe(' #build Working sally working sam idle ');
  });

  it('says so when the Work item has no changes', () => {
    useDiff.mockReturnValue({ data: [] });
    const markup = renderToStaticMarkup(
      <SpaceCard space={space} workItem={workItem} members={members} working={[]} waiting={[]} />,
    );
    expect(text(markup)).toContain('Changes No changes yet');
  });
});

describe('spaceState', () => {
  const open = { closedAt: null, paused: '' };

  it('picks the first of wrapped, paused, failed, working, waiting, idle', () => {
    expect(spaceState({ closedAt: 'x', paused: 'cap' }, [1], ['waiting'])).toEqual({
      status: 'done',
      label: 'Wrapped',
    });
    expect(spaceState({ closedAt: null, paused: 'token budget' }, [1], ['waiting'])).toEqual({
      status: 'paused',
      label: 'Paused: token budget',
    });
    expect(spaceState(open, [1], ['waiting', 'failed'])).toEqual({
      status: 'failed',
      label: 'A turn failed',
    });
    expect(spaceState(open, [1], ['waiting'])).toEqual({
      status: 'working',
      label: 'Working',
    });
    expect(spaceState(open, [], ['waiting'])).toEqual({
      status: 'needs-you',
      label: 'Waiting for you',
    });
    expect(spaceState(open, [], [])).toEqual({ status: 'asleep', label: 'Idle' });
  });
});
