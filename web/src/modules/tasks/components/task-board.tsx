import { useState } from 'react';
import type { DragEvent } from 'react';
import { useUpdateTask } from '@/modules/tasks/api/update-task';
import type { Feature, Task, TaskStatus } from '@/types/api';

const COLUMNS: { status: TaskStatus; label: string }[] = [
  { status: 'todo', label: 'Todo' },
  { status: 'doing', label: 'Doing' },
  { status: 'done', label: 'Done' },
];

const DONE_SHOWN = 5;

type Props = {
  tasks: Task[];
  features: Feature[];
};

export function TaskBoard({ tasks, features }: Props) {
  const update = useUpdateTask();
  const [allDone, setAllDone] = useState(false);

  function onDrop(e: DragEvent<HTMLElement>, status: TaskStatus) {
    e.preventDefault();
    const id = Number(e.dataTransfer.getData('text/plain'));
    const task = tasks.find((t) => t.id === id);
    if (task && task.status !== status) update.mutate({ task, status });
  }

  return (
    <div className="space-y-2">
      {update.isError && <p className="text-sm text-danger">{update.error.message}</p>}
      <div className="grid grid-cols-1 items-start gap-4 md:grid-cols-3 md:overflow-x-auto">
        {COLUMNS.map((col) => {
          const inColumn = tasks.filter((t) => t.status === col.status);
          const hidden = col.status === 'done' && !allDone ? inColumn.length - DONE_SHOWN : 0;
          return (
            <section
              key={col.status}
              role="group"
              aria-label={col.label}
              className="flex flex-col gap-2.5 rounded-card bg-sidebar p-3.5"
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => onDrop(e, col.status)}
            >
              <h2 className="flex items-center gap-2 px-1 py-0.5 text-sm font-normal">
                {col.label}
                <span className="text-[12.5px] text-muted">{inColumn.length}</span>
              </h2>
              {(hidden > 0 ? inColumn.slice(0, DONE_SHOWN) : inColumn).map((task) => {
                const done = task.status === 'done';
                const feature = features.find((f) => f.id === task.featureId);
                return (
                  <article
                    key={task.id}
                    draggable
                    onDragStart={(e) => e.dataTransfer.setData('text/plain', String(task.id))}
                    className={`flex flex-col gap-2 rounded-menu px-3.5 py-3 ${done ? 'bg-main text-muted' : 'bg-card'}`}
                  >
                    <p className={done ? 'line-through' : ''}>{task.title}</p>
                    <div className="flex items-center gap-2 text-xs text-muted">
                      {feature && (
                        <span className="flex h-5 items-center rounded-pill bg-track px-2">
                          {feature.name}
                        </span>
                      )}
                      <select
                        aria-label={`Move ${task.title}`}
                        value={task.status}
                        onChange={(e) =>
                          update.mutate({ task, status: e.target.value as TaskStatus })
                        }
                        className="ml-auto h-11 rounded-field md:h-6 bg-transparent px-1 text-xs text-muted outline-none focus-visible:ring-3 focus-visible:ring-ring"
                      >
                        {COLUMNS.map((c) => (
                          <option key={c.status} value={c.status}>
                            {c.label}
                          </option>
                        ))}
                      </select>
                    </div>
                  </article>
                );
              })}
              {inColumn.length === 0 && (
                <div className="rounded-menu border border-dashed border-line p-4 text-center text-[12.5px] text-muted">
                  Drop a card here
                </div>
              )}
              {hidden > 0 && (
                <button
                  type="button"
                  className="min-h-11 self-start p-1 text-[12.5px] text-muted"
                  onClick={() => setAllDone(true)}
                >
                  Show {hidden} more
                </button>
              )}
            </section>
          );
        })}
      </div>
    </div>
  );
}
