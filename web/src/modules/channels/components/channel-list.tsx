import { Link } from 'react-router';
import type { Channel } from '@/types/api';
import { StatusDot } from '@/components/status-dot';

export function ChannelList({ channels }: { channels: Channel[] }) {
  if (channels.length === 0) return <p className="text-sm text-muted">No channels.</p>;

  return (
    <ul className="text-[13px]">
      {channels.map((c) => (
        <li key={c.id}>
          <Link
            to={`/channels/${c.id}`}
            className="flex min-h-10 items-center gap-2.5 text-soft hover:text-fg"
          >
            <StatusDot status={c.paused ? 'paused' : 'asleep'} />
            {c.name}
            {c.paused && <span className="ml-auto text-xs text-paused">paused</span>}
          </Link>
        </li>
      ))}
    </ul>
  );
}
