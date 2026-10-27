import { useState } from 'react';
import { Link } from 'react-router';
import { useReorderQueue } from '@/modules/features/api/reorder-queue';
import { dropAction } from '@/modules/features/utils/board-drop';
import { statusLabel } from '@/modules/features/utils/status-label';
import type { Space, WorkItem } from '@/types/api';

type Props = { items: WorkItem[]; spaces: Space[] };

const panel = 'flex flex-col gap-2 rounded-card bg-sidebar p-3';
const row = 'flex min-h-11 items-start gap-2.5 rounded-xl bg-card p-3 hover:bg-hover/50';

export function TeamQueue({ items, spaces }: Props) {
  const reorder = useReorderQueue();
  const [dragging, setDragging] = useState<WorkItem | null>(null);
  const active = items.find((i) => i.status === 'in_progress' || i.status === 'needs_you');
  const queued = items
    .filter((i) => i.status === 'queued')
    .sort((a, b) => (a.queuePosition ?? 0) - (b.queuePosition ?? 0));
  const space = spaces
    .filter((s) => s.workItemId === active?.id && s.closedAt === null)
    .sort((a, b) => b.id - a.id)[0];

  function drop(onto: WorkItem) {
    const action = dragging && dropAction(dragging, 'queued', onto);
    setDragging(null);
    if (dragging && action?.kind === 'reorder') {
      reorder.mutate({ id: dragging.id, position: action.position });
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <section aria-label="Now" className={panel}>
        <h2 className="px-1 text-[13px] text-soft first-letter:uppercase">
          {active ? statusLabel(active.status) : 'Idle'}
        </h2>
        {active ? (
          <Link to={space ? `/spaces/${space.id}` : `/features/${active.id}`} className={row}>
            <span className="min-w-0 flex-1">
              <span className="block text-[14.5px] break-words">{active.title}</span>
              <span className="block text-xs text-muted">
                {active.type} · {space ? 'open its Space' : 'no open Space'}
              </span>
            </span>
          </Link>
        ) : (
          <p className="px-1 text-sm text-muted">Nothing is running for this team.</p>
        )}
      </section>
      <section aria-label="Queued" className={panel}>
        <h2 className="px-1 text-[13px] text-soft">
          Queued <span className="text-muted">{queued.length}</span>
        </h2>
        {reorder.isError && (
          <p role="alert" className="text-sm text-danger">
            {reorder.error.message}
          </p>
        )}
        {queued.length === 0 && <p className="px-1 text-sm text-muted">Nothing is queued.</p>}
        <ul className="flex flex-col gap-2">
          {queued.map((item) => (
            <li key={item.id} onDragOver={(e) => e.preventDefault()} onDrop={() => drop(item)}>
              <Link
                to={`/features/${item.id}`}
                draggable
                onDragStart={() => {
                  reorder.reset();
                  setDragging(item);
                }}
                onDragEnd={() => setDragging(null)}
                className={`${row} [-webkit-user-drag:element] ${dragging?.id === item.id ? 'opacity-50' : ''}`}
              >
                <span
                  aria-label={`Queue position ${item.queuePosition}`}
                  className="mt-px flex size-5 flex-none items-center justify-center rounded-full bg-bubble text-[11px] text-soft"
                >
                  {item.queuePosition}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block text-[14.5px] break-words">{item.title}</span>
                  <span className="block text-xs text-muted">{item.type}</span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
