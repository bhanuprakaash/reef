import { Author } from '@/modules/channels/components/author';
import { Bubble } from '@/modules/channels/components/bubble';
import { Composer } from '@/modules/channels/components/composer';
import type { Thread } from '@/modules/channels/utils/group-threads';
import { Button } from '@/components/button';

type Props = {
  channelId: number;
  thread: Thread;
  lead: string;
  roles: string[];
  faces: Record<string, string | undefined>;
  running: boolean;
  paused: boolean;
  reason: string;
  closed: boolean;
  working: string;
  onClose: () => void;
};

export function ThreadPane({
  channelId,
  thread,
  lead,
  roles,
  faces,
  running,
  paused,
  reason,
  closed,
  working,
  onClose,
}: Props) {
  const { root, replies } = thread;
  const owner = roles.includes(root.author) ? root.author : lead;
  const time = new Date(root.createdAt).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  });

  return (
    <aside aria-label="Thread" className="flex min-h-0 flex-1 flex-col bg-sidebar max-md:bg-main">
      <div className="flex min-h-14 items-center gap-2 border-b border-hairline py-2 pl-[18px] pr-3 max-md:pl-2">
        <h2 className="text-[15px] font-normal">Thread</h2>
        <span className="text-[12.5px] text-muted">
          owned by <span className="capitalize">{owner}</span>
        </span>
        <Button
          variant="ghost"
          size="icon"
          type="button"
          aria-label="Close thread"
          className="ml-auto max-md:order-first max-md:ml-0"
          onClick={onClose}
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path className="max-md:hidden" d="M6 6l12 12M18 6L6 18" />
            <path className="md:hidden" d="M15 5l-7 7 7 7" />
          </svg>
        </Button>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto px-[18px] py-4">
        <ul>
          <li className="flex flex-col items-start gap-2.5">
            <Author name={root.author} face={faces[root.author]} time={time} />
            <Bubble own={root.author === 'user'}>{root.body}</Bubble>
          </li>
        </ul>
        {replies.length > 0 && (
          <div className="my-1 flex items-center gap-2.5 text-xs text-muted">
            <span className="h-px flex-1 bg-hairline" />
            {replies.length} {replies.length === 1 ? 'reply' : 'replies'}
            <span className="h-px flex-1 bg-hairline" />
          </div>
        )}
        <ul className="flex flex-col gap-2.5">
          {replies.map((m, i) => (
            <li
              key={m.id}
              className={`flex flex-col gap-2.5 ${m.author === 'user' ? 'items-end' : 'items-start'}`}
            >
              {m.author !== 'user' && replies[i - 1]?.author !== m.author && (
                <Author name={m.author} face={faces[m.author]} />
              )}
              <Bubble own={m.author === 'user'}>{m.body}</Bubble>
            </li>
          ))}
        </ul>
        <p className="pt-1 text-[12.5px] text-muted">
          Your reply goes to <span className="capitalize">{owner}</span>, who owns this thread.
        </p>
      </div>
      {!closed && (
        <div className="px-3.5 pb-5 pt-2.5">
          <Composer
            channelId={channelId}
            running={running}
            paused={paused}
            parent={root.id}
            reason={reason}
            roles={roles}
            faces={faces}
            working={working}
          />
        </div>
      )}
    </aside>
  );
}
