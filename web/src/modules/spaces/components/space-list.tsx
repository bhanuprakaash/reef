import { Link } from 'react-router';
import type { Space } from '@/types/api';
import { EmptyState } from '@/components/empty-state';
import { StatusDot } from '@/components/status-dot';

function Spaces({ spaces }: { spaces: Space[] }) {
  return (
    <ul className="text-[13px]">
      {spaces.map((c) => (
        <li key={c.id}>
          <Link
            to={`/spaces/${c.id}`}
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

export function SpaceList({ spaces }: { spaces: Space[] }) {
  if (spaces.length === 0) {
    return (
      <EmptyState
        title="No Spaces yet"
        body="Start one to talk to the team."
        face="jellyfish"
        compact
      />
    );
  }

  const open = spaces.filter((c) => c.closedAt === null);
  const wrapped = spaces.filter((c) => c.closedAt !== null);
  return (
    <>
      {open.length > 0 && <Spaces spaces={open} />}
      {wrapped.length > 0 && (
        <details>
          <summary className="cursor-pointer py-2 text-xs text-muted">
            Wrapped ({wrapped.length})
          </summary>
          <Spaces spaces={wrapped} />
        </details>
      )}
    </>
  );
}
