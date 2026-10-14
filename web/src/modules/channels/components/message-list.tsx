import type { Thread } from '@/modules/channels/utils/group-threads';
import { latestMessageAnchor } from '@/utils/anchors';
import { buttonClass, mutedClass } from '@/utils/classes';

type Props = { threads: Thread[]; onOpen: (rootId: number) => void };

export function MessageList({ threads, onOpen }: Props) {
  if (threads.length === 0) return <p className={mutedClass}>No messages yet.</p>;

  const newest = Math.max(...threads.map(({ root, replies }) => (replies.at(-1) ?? root).id));

  return (
    <ul className="space-y-3">
      {threads.map(({ root, replies }) => (
        <li
          key={root.id}
          id={(replies.at(-1) ?? root).id === newest ? latestMessageAnchor : undefined}
        >
          <div className="text-xs font-medium uppercase text-neutral-500">{root.author}</div>
          <p className="whitespace-pre-wrap text-sm">{root.body}</p>
          <button type="button" className={`${buttonClass} mt-1`} onClick={() => onOpen(root.id)}>
            {replies.length === 0
              ? 'Reply'
              : `${replies.length} ${replies.length === 1 ? 'reply' : 'replies'}`}
          </button>
        </li>
      ))}
    </ul>
  );
}
