import { Author } from '@/modules/channels/components/author';
import { Composer } from '@/modules/channels/components/composer';
import type { Thread } from '@/modules/channels/utils/group-threads';
import { Button } from '@/components/button';

type Props = {
  channelId: number;
  thread: Thread;
  running: boolean;
  paused: boolean;
  onClose: () => void;
};

export function ThreadPane({ channelId, thread, running, paused, onClose }: Props) {
  return (
    <aside aria-label="Thread" className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium">Thread</h2>
        <Button variant="ghost" type="button" onClick={onClose}>
          Close
        </Button>
      </div>
      <ul className="space-y-3">
        {[thread.root, ...thread.replies].map((m) => (
          <li key={m.id}>
            <Author name={m.author} />
            <p className="whitespace-pre-wrap text-sm">{m.body}</p>
          </li>
        ))}
      </ul>
      <Composer channelId={channelId} running={running} paused={paused} parent={thread.root.id} />
    </aside>
  );
}
