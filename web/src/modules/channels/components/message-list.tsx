import type { Thread } from '@/modules/channels/utils/group-threads';
import { buttonClass, mutedClass } from '@/utils/classes';

type Props = { threads: Thread[]; onOpen: (rootId: number) => void };

export function MessageList({ threads, onOpen }: Props) {
  if (threads.length === 0) return <p className={mutedClass}>No messages yet.</p>;

  return (
    <ul className="space-y-3">
      {threads.map(({ root, replies }) => (
        <li key={root.id}>
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
