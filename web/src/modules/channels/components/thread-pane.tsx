import { Composer } from '@/modules/channels/components/composer';
import type { Thread } from '@/modules/channels/utils/group-threads';
import { buttonClass } from '@/utils/classes';

type Props = {
  channelId: number;
  thread: Thread;
  running: boolean;
  paused: boolean;
  onClose: () => void;
};

export function ThreadPane({ channelId, thread, running, paused, onClose }: Props) {
  return (
    <aside
      aria-label="Thread"
      className="w-80 shrink-0 space-y-3 self-start rounded border border-neutral-300 p-3 dark:border-neutral-700"
    >
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium">Thread</h2>
        <button type="button" className={buttonClass} onClick={onClose}>
          Close
        </button>
      </div>
      <ul className="space-y-3">
        {[thread.root, ...thread.replies].map((m) => (
          <li key={m.id}>
            <div className="text-xs font-medium uppercase text-neutral-500">{m.author}</div>
            <p className="whitespace-pre-wrap text-sm">{m.body}</p>
          </li>
        ))}
      </ul>
      <Composer channelId={channelId} running={running} paused={paused} parent={thread.root.id} />
    </aside>
  );
}
