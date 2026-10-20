import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import type { ChannelPreview, Feature, FileDiff } from '@/types/api';
import { ChannelCard, channelState } from './channel-hovercard';

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

const channel: ChannelPreview = {
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

describe('ChannelCard', () => {
  it('shows the branch, the changes and who is working', () => {
    useDiff.mockReturnValue({ data: files });
    const markup = renderToStaticMarkup(
      <ChannelCard
        channel={channel}
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
      <ChannelCard
        channel={{ ...channel, featureId: null }}
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
      <ChannelCard channel={channel} feature={feature} faces={faces} working={[]} waiting={[]} />,
    );
    expect(text(markup)).toContain('Changes No changes yet');
  });
});

describe('channelState', () => {
  const open = { closedAt: null, paused: '' };

  it('picks the first of wrapped, paused, approval, failed, working, waiting, idle', () => {
    expect(channelState({ closedAt: 'x', paused: 'cap' }, ['lead'], ['approval'])).toEqual({
      status: 'done',
      label: 'Wrapped',
    });
    expect(
      channelState({ closedAt: null, paused: 'token budget' }, ['lead'], ['approval']),
    ).toEqual({
      status: 'paused',
      label: 'Paused: token budget',
    });
    expect(channelState(open, ['lead'], ['waiting', 'approval'])).toEqual({
      status: 'needs-you',
      label: 'Waiting for your approval',
    });
    expect(channelState(open, ['lead'], ['waiting', 'failed'])).toEqual({
      status: 'failed',
      label: 'A turn failed',
    });
    expect(channelState(open, ['lead'], ['waiting'])).toEqual({
      status: 'working',
      label: 'Working',
    });
    expect(channelState(open, [], ['waiting'])).toEqual({
      status: 'needs-you',
      label: 'Waiting for you',
    });
    expect(channelState(open, [], [])).toEqual({ status: 'asleep', label: 'Idle' });
  });
});
