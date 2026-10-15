import type { Thread } from '@/modules/channels/utils/group-threads';
import { latestMessageAnchor } from '@/utils/anchors';
import { Button } from '@/components/button';

type Props = { threads: Thread[]; onOpen: (rootId: number) => void };

export function MessageList({ threads, onOpen }: Props) {
  if (threads.length === 0) return <p className="text-sm text-muted">No messages yet.</p>;

  const newest = Math.max(...threads.map(({ root, replies }) => (replies.at(-1) ?? root).id));

  return (
    <ul className="space-y-3">
      {threads.map(({ root, replies }) => (
        <li
          key={root.id}
          id={(replies.at(-1) ?? root).id === newest ? latestMessageAnchor : undefined}
        >
          <div className="text-xs font-medium uppercase text-muted">{root.author}</div>
          <p className="whitespace-pre-wrap text-sm">{root.body}</p>
          <Button type="button" className="mt-1" onClick={() => onOpen(root.id)}>
            {replies.length === 0
              ? 'Reply'
              : `${replies.length} ${replies.length === 1 ? 'reply' : 'replies'}`}
          </Button>
        </li>
      ))}
    </ul>
  );
}
