import { useState } from 'react';
import { Link } from 'react-router';
import { useMoveWorkItem } from '@/modules/work-items/api/move-work-item';
import { useReorderQueue } from '@/modules/work-items/api/reorder-queue';
import { COLUMNS, dropAction } from '@/modules/work-items/utils/board-drop';
import type { Team, WorkItem, WorkItemStatus } from '@/types/api';

type Props = { items: WorkItem[]; teams: Team[] };

const byPosition = (a: WorkItem, b: WorkItem) => (a.queuePosition ?? 0) - (b.queuePosition ?? 0);

export function Board({ items, teams }: Props) {
  const move = useMoveWorkItem();
  const reorder = useReorderQueue();
  const [dragging, setDragging] = useState<WorkItem | null>(null);
  const [refusal, setRefusal] = useState<string | null>(null);
  const teamName = (id: number | null) => teams.find((t) => t.id === id)?.name;

  function drop(column: WorkItemStatus, onto?: WorkItem) {
    setDragging(null);
    const action = dragging && dropAction(dragging, column, onto);
    if (!dragging || !action) return;
    if (action.kind === 'refuse') setRefusal(action.reason);
    else if (action.kind === 'move') move.mutate({ id: dragging.id, status: action.status });
    else reorder.mutate({ id: dragging.id, position: action.position });
  }

  function card(item: WorkItem) {
    const team = teamName(item.teamId);
    return (
      <li key={item.id} onDrop={(e) => (e.stopPropagation(), drop(item.status, item))}>
        <Link
          to={`/work-items/${item.id}`}
          draggable
          onDragStart={() => {
            setRefusal(null);
            move.reset();
            reorder.reset();
            setDragging(item);
          }}
          onDragEnd={() => setDragging(null)}
          className={`flex min-h-11 items-start gap-2.5 [-webkit-user-drag:element] rounded-xl bg-card p-3 hover:bg-hover/50 ${dragging?.id === item.id ? 'opacity-50' : ''}`}
        >
          {item.status === 'queued' && item.queuePosition !== null && (
            <span
              aria-label={`Queue position ${item.queuePosition}`}
              className="mt-px flex size-5 flex-none items-center justify-center rounded-full bg-bubble text-[11px] text-soft"
            >
              {item.queuePosition}
            </span>
          )}
          <span className="min-w-0 flex-1">
            <span className="block text-[14.5px] break-words">{item.title}</span>
            <span className="block text-xs text-muted">
              {item.type}
              {team && ` · ${team}`}
            </span>
          </span>
        </Link>
      </li>
    );
  }

  function queued(column: WorkItem[]) {
    const teamIds = [...new Set(column.map((i) => i.teamId))].sort((a, b) =>
      (teamName(a) ?? '~').localeCompare(teamName(b) ?? '~'),
    );
    return teamIds.map((id) => (
      <div key={id ?? 'none'} role="group" aria-label={`${teamName(id) ?? 'No Team'} queue`}>
        <h3 className="px-1 pb-1.5 text-xs text-muted">{teamName(id) ?? 'No Team'}</h3>
        <ul className="flex flex-col gap-2">
          {column
            .filter((i) => i.teamId === id)
            .sort(byPosition)
            .map(card)}
        </ul>
      </div>
    ));
  }

  const problem = refusal ?? move.error?.message ?? reorder.error?.message;

  return (
    <>
      {problem && (
        <p role="alert" className="mb-3 text-sm text-danger">
          {problem}
        </p>
      )}
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:overflow-x-auto md:pb-2">
        {COLUMNS.map(({ status, label }) => {
          const column = items.filter((i) => i.status === status);
          const target = dragging && dropAction(dragging, status)?.kind === 'move';
          return (
            <section
              key={status}
              aria-label={label}
              onDragOver={(e) => e.preventDefault()}
              onDrop={() => drop(status)}
              className={`flex flex-col gap-2 rounded-card bg-sidebar p-3 md:w-[248px] md:shrink-0 ${target ? 'ring-1 ring-working/50' : ''}`}
            >
              <h2 className="px-1 text-[13px] text-soft">
                {label} <span className="text-muted">{column.length}</span>
              </h2>
              {status === 'queued' ? (
                queued(column)
              ) : (
                <ul className="flex flex-col gap-2">{column.map(card)}</ul>
              )}
            </section>
          );
        })}
      </div>
    </>
  );
}
