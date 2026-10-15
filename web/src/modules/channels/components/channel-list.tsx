import { Link } from 'react-router';
import { useTeams } from '@/lib/api/get-teams';
import type { Channel } from '@/types/api';
import { GroupAvatar } from '@/components/creature';

export function ChannelList({ channels }: { channels: Channel[] }) {
  const teams = useTeams();

  if (channels.length === 0) return <p className="text-sm text-muted">No channels.</p>;

  return (
    <ul className="space-y-1 text-sm">
      {channels.map((c) => {
        const roles = teams.data?.find((t) => t.name === c.team)?.roles.map((r) => r.name);
        return (
          <li key={c.id}>
            <Link to={`/channels/${c.id}`} className="flex items-center gap-2 hover:underline">
              <GroupAvatar roles={roles ?? [c.role]} size={30} />
              {c.name}
            </Link>
          </li>
        );
      })}
    </ul>
  );
}
