import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { SpacePreview, Feature, FileDiff } from '@/types/api';
import { SpaceCard, spaceState } from './space-hovercard';

const { useDiff } = vi.hoisted(() => ({ useDiff: vi.fn() }));
vi.mock('@/modules/features/api/get-diff', () => ({ useDiff }));

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

const space: SpacePreview = {
  id: 4,
  projectId: 1,
  featureId: 2,
  name: 'build',
  team: 'full',
  role: 'lead',
  backend: 'claude',
  permissionMode: 'default',
  paused: '',
  createdAt: '',
  closedAt: null,
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

const faces = { lead: undefined, developer: undefined };
const text = (markup: string) => markup.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ');

describe('SpaceCard', () => {
  it('shows the branch, the changes and who is working', () => {
    useDiff.mockReturnValue({ data: files });
    const markup = renderToStaticMarkup(
      <SpaceCard
        space={space}
        feature={feature}
        faces={faces}
        working={['developer']}
        waiting={[]}
      />,
    );
    expect(text(markup)).toBe(
      ' #build Working Branch feature/login Changes 2 files +3 −1 lead idle developer working ',
    );
    expect(useDiff).toHaveBeenCalledWith(2);
  });

  it('has no branch or changes for a quick session and does not fetch a diff', () => {
    useDiff.mockClear();
    const markup = renderToStaticMarkup(
      <SpaceCard
        space={{ ...space, featureId: null }}
        feature={undefined}
        faces={faces}
        working={[]}
        waiting={[]}
      />,
    );
    expect(text(markup)).toBe(' #build Idle lead idle developer idle ');
    expect(useDiff).not.toHaveBeenCalled();
  });

  it('says so when the feature has no changes', () => {
    useDiff.mockReturnValue({ data: [] });
    const markup = renderToStaticMarkup(
      <SpaceCard space={space} feature={feature} faces={faces} working={[]} waiting={[]} />,
    );
    expect(text(markup)).toContain('Changes No changes yet');
  });
});

describe('spaceState', () => {
  const open = { closedAt: null, paused: '' };

  it('picks the first of wrapped, paused, approval, failed, working, waiting, idle', () => {
    expect(spaceState({ closedAt: 'x', paused: 'cap' }, ['lead'], ['approval'])).toEqual({
      status: 'done',
      label: 'Wrapped',
    });
    expect(spaceState({ closedAt: null, paused: 'token budget' }, ['lead'], ['approval'])).toEqual({
      status: 'paused',
      label: 'Paused: token budget',
    });
    expect(spaceState(open, ['lead'], ['waiting', 'approval'])).toEqual({
      status: 'needs-you',
      label: 'Waiting for your approval',
    });
    expect(spaceState(open, ['lead'], ['waiting', 'failed'])).toEqual({
      status: 'failed',
      label: 'A turn failed',
    });
    expect(spaceState(open, ['lead'], ['waiting'])).toEqual({
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
