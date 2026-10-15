import { Link } from 'react-router';
import type { Channel } from '@/types/api';

export function ChannelList({ channels }: { channels: Channel[] }) {
  if (channels.length === 0) return <p className="text-sm text-muted">No channels.</p>;

  return (
    <ul className="space-y-1 text-sm">
      {channels.map((c) => (
        <li key={c.id}>
          <Link to={`/channels/${c.id}`} className="hover:underline">
            {c.name}
          </Link>
        </li>
      ))}
    </ul>
  );
}
