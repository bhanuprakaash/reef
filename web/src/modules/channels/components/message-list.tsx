import { Fragment } from 'react';
import type { Thread } from '@/modules/channels/utils/group-threads';
import { Author } from '@/modules/channels/components/author';
import { Bubble } from '@/modules/channels/components/bubble';
import { PullRequestIcon } from '@/modules/channels/components/pull-request-icon';
import { showTimestamp, timestampLabel } from '@/modules/channels/utils/message-time';
import { checkResult, pullRequestNote } from '@/modules/channels/utils/pull-request';
import { latestMessageAnchor } from '@/utils/anchors';
import { Button } from '@/components/button';
import { Creature, GroupAvatar } from '@/components/creature';
import { StatusDot } from '@/components/status-dot';

type Props = { threads: Thread[]; onOpen: (rootId: number) => void };

const isTeammate = (author: string) => author !== 'user' && author !== 'reef';

export function MessageList({ threads, onOpen }: Props) {
  if (threads.length === 0) return <p className="text-sm text-muted">No messages yet.</p>;

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
        return (
          <Fragment key={root.id}>
            {stamped && (
              <li className="py-1 text-center text-[13px] text-faint">
                {timestampLabel(root.createdAt, previous?.createdAt, now)}
              </li>
            )}
            {joined && (
              <li className="flex items-center justify-center gap-2 py-2.5 text-sm text-muted">
                Joined
                <Creature role={root.author} size={18} />
                <span className="capitalize text-fg">{root.author}</span>
              </li>
            )}
            <li
              id={(replies.at(-1) ?? root).id === newest ? latestMessageAnchor : undefined}
              className={`flex flex-col gap-2.5 ${root.author === 'user' ? 'items-end' : 'items-start'} ${named ? 'mt-2.5' : ''}`}
            >
              {pullRequest ? (
                <p className="flex w-full items-center justify-center gap-2 py-2 text-sm text-muted">
                  <PullRequestIcon />
                  GitHub
                  <a href={pullRequest.url} target="_blank" rel="noreferrer" className="text-fg">
                    PR #{pullRequest.number} {pullRequest.state}
                  </a>
                </p>
              ) : root.author === 'reef' ? (
                <p
                  className={`flex w-full items-center justify-center gap-2 whitespace-pre-wrap text-center text-sm ${check ? 'py-1' : 'py-2.5'} ${check === 'failed' ? 'text-danger' : 'text-muted'}`}
                >
                  {check && <StatusDot status={check === 'failed' ? 'failed' : 'done'} />}
                  {root.body}
                </p>
              ) : (
                <>
                  {named && <Author name={root.author} />}
                  <Bubble own={root.author === 'user'}>{root.body}</Bubble>
                </>
              )}
              {(root.author !== 'reef' || replies.length > 0) && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="gap-2 px-1"
                  onClick={() => onOpen(root.id)}
                >
                  {repliers.length > 0 && <GroupAvatar roles={repliers} size={20} />}
                  {replies.length === 0
                    ? 'Reply'
                    : `${replies.length} ${replies.length === 1 ? 'reply' : 'replies'} in thread`}
                </Button>
              )}
            </li>
          </Fragment>
        );
      })}
    </ul>
  );
}
