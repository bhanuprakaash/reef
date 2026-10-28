import { Fragment } from 'react';
import type { Chain } from '@/modules/spaces/utils/direct-chains';
import type { Thread } from '@/modules/spaces/utils/group-threads';
import type { Turn } from '@/types/api';
import { Author } from '@/modules/spaces/components/author';
import { Bubble } from '@/modules/spaces/components/bubble';
import { CheckpointButton } from '@/modules/spaces/components/checkpoint-button';
import { CoordinationItem } from '@/modules/spaces/components/coordination-item';
import { PullRequestIcon } from '@/modules/spaces/components/pull-request-icon';
import { checkpointAnchor } from '@/modules/spaces/utils/checkpoint-anchor';
import { showTimestamp, timestampLabel } from '@/modules/spaces/utils/message-time';
import { checkResult, pullRequestNote } from '@/modules/spaces/utils/pull-request';
import { latestMessageAnchor } from '@/utils/anchors';
import { Button } from '@/components/button';
import { Creature, GroupAvatar, type Faces } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';
import { StatusDot } from '@/components/status-dot';

type Props = {
  spaceId: number;
  threads: Thread[];
  chains: Chain[];
  faces: Faces;
  openId?: number;
  openChainId?: number;
  running?: boolean;
  turns: Turn[];
  checkpoints: Turn[];
  onOpen: (rootId: number) => void;
  onOpenChain: (chainId: number) => void;
};

const isTeammate = (author: string) => author !== 'user' && author !== 'reef';

const icon = {
  width: 15,
  height: 15,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.8,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
  'aria-hidden': true,
} as const;

export function MessageList({
  spaceId,
  threads,
  chains,
  faces,
  openId,
  openChainId,
  running = false,
  turns,
  checkpoints,
  onOpen,
  onOpenChain,
}: Props) {
  if (threads.length === 0) {
    return (
      <div className="py-16">
        <EmptyState
          title="Quiet water"
          body="Say what you want to build. Lead answers first."
          face="jellyfish"
        />
      </div>
    );
  }

  const newest = Math.max(...threads.map(({ root, replies }) => (replies.at(-1) ?? root).id));
  const firstId = new Map<string, number>();
  for (const { root, replies } of threads) {
    for (const m of [root, ...replies]) {
      firstId.set(m.author, Math.min(m.id, firstId.get(m.author) ?? m.id));
    }
  }
  const founder = Math.min(
    ...[...firstId].filter(([author]) => isTeammate(author)).map(([, id]) => id),
  );
  const now = new Date();
  const anchors = checkpointAnchor(turns, checkpoints, threads);
  const before: Chain[][] = threads.map(() => []);
  const after: Chain[] = [];
  for (const chain of chains) {
    const at = Date.parse(chain.messages[0].createdAt);
    const i = threads.findIndex(({ root }) => Date.parse(root.createdAt) > at);
    (i < 0 ? after : before[i]).push(chain);
  }
  const item = (chain: Chain) => (
    <CoordinationItem
      key={`direct-${chain.id}`}
      chain={chain}
      faces={faces}
      open={chain.id === openChainId}
      onOpen={() => onOpenChain(chain.id)}
    />
  );

  return (
    <ul className="flex flex-col gap-2.5">
      {threads.map(({ root, replies }, i) => {
        const previous = threads[i - 1]?.root;
        const stamped = showTimestamp(previous?.createdAt, root.createdAt);
        const joined =
          isTeammate(root.author) && firstId.get(root.author) === root.id && root.id !== founder;
        const named =
          isTeammate(root.author) && (stamped || joined || previous?.author !== root.author);
        const pullRequest = pullRequestNote(root);
        const check = checkResult(root);
        const repliers = [...new Set(replies.map((r) => r.author).filter(isTeammate))];
        const anchor = anchors.get(root.id);
        return (
          <Fragment key={root.id}>
            {before[i].map(item)}
            {stamped && (
              <li className="py-1 text-center text-[13px] text-faint">
                {timestampLabel(root.createdAt, previous?.createdAt, now)}
              </li>
            )}
            {joined && (
              <li className="flex items-center justify-center gap-2 py-2.5 text-sm text-muted">
                Joined
                <Creature look={faces[root.author]} size={18} />
                <span className="capitalize text-fg">{root.author}</span>
              </li>
            )}
            <li
              id={(replies.at(-1) ?? root).id === newest ? latestMessageAnchor : undefined}
              className={`flex flex-col gap-2.5 ${root.author === 'user' ? 'items-end' : 'items-start'} ${named ? 'mt-2.5' : ''}`}
            >
              {pullRequest || root.author === 'reef' ? (
                <>
                  {pullRequest ? (
                    <p className="flex w-full items-center justify-center gap-2 py-2 text-sm text-muted">
                      <PullRequestIcon />
                      GitHub
                      <a
                        href={pullRequest.url}
                        target="_blank"
                        rel="noreferrer"
                        className="text-fg"
                      >
                        PR #{pullRequest.number} {pullRequest.state}
                      </a>
                    </p>
                  ) : (
                    <p
                      className={`flex w-full items-center justify-center gap-2 whitespace-pre-wrap text-center text-sm ${check ? 'py-1' : 'py-2.5'} ${check === 'failed' ? 'text-danger' : 'text-muted'}`}
                    >
                      {check && <StatusDot status={check === 'failed' ? 'failed' : 'done'} />}
                      {root.body}
                    </p>
                  )}
                  {(root.author !== 'reef' || replies.length > 0) && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className={`gap-2 px-1 ${root.id === openId ? 'bg-card' : ''}`}
                      onClick={() => onOpen(root.id)}
                    >
                      {repliers.length > 0 && (
                        <GroupAvatar roles={repliers} faces={faces} size={20} />
                      )}
                      {replies.length === 0
                        ? 'Reply'
                        : `${replies.length} ${replies.length === 1 ? 'reply' : 'replies'} in thread`}
                    </Button>
                  )}
                </>
              ) : (
                <>
                  {named && <Author name={root.author} look={faces[root.author]} />}
                  <Bubble own={root.author === 'user'} ringed={root.id === openId}>
                    {root.body}
                  </Bubble>
                  <div className="group/msg flex items-center gap-1.5">
                    {replies.length > 0 && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className={`gap-2 px-1 ${root.id === openId ? 'bg-card' : ''}`}
                        onClick={() => onOpen(root.id)}
                      >
                        <GroupAvatar roles={repliers} faces={faces} size={20} />
                        {replies.length} {replies.length === 1 ? 'reply' : 'replies'} in thread
                      </Button>
                    )}
                    {(anchor || replies.length === 0) && (
                      <div
                        className={`flex items-center gap-1 ${root.id === openId && replies.length === 0 ? '' : 'opacity-0 transition-opacity group-hover/msg:opacity-100 focus-within:opacity-100 motion-reduce:transition-none'}`}
                      >
                        {anchor && (
                          <CheckpointButton spaceId={spaceId} turn={anchor} running={running} />
                        )}
                        {replies.length === 0 && (
                          <button
                            type="button"
                            aria-label="Reply in thread"
                            title="Reply in thread"
                            className="flex size-7 items-center justify-center rounded-field text-soft hover:bg-card"
                            onClick={() => onOpen(root.id)}
                          >
                            <svg {...icon}>
                              <path d="M9 17l-5-5 5-5" />
                              <path d="M20 18v-2a4 4 0 0 0-4-4H4" />
                            </svg>
                          </button>
                        )}
                      </div>
                    )}
                  </div>
                </>
              )}
            </li>
          </Fragment>
        );
      })}
      {after.map(item)}
    </ul>
  );
}
