import type { DragEvent } from 'react';
import { useUpdateTask } from '@/modules/tasks/api/update-task';
import type { Task, TaskStatus } from '@/types/api';
import { Select } from '@/components/select';

const COLUMNS: { status: TaskStatus; label: string }[] = [
  { status: 'todo', label: 'Todo' },
  { status: 'doing', label: 'Doing' },
  { status: 'done', label: 'Done' },
];

type Props = {
  tasks: Task[];
  context?: (task: Task) => string;
};

export function TaskBoard({ tasks, context }: Props) {
  const update = useUpdateTask();

  function onDrop(e: DragEvent<HTMLDivElement>, status: TaskStatus) {
    e.preventDefault();
    const id = Number(e.dataTransfer.getData('text/plain'));
    const task = tasks.find((t) => t.id === id);
    if (task && task.status !== status) update.mutate({ task, status });
  }

  return (
    <div className="space-y-2">
      {update.isError && <p className="text-sm text-danger">{update.error.message}</p>}
      <div className="grid grid-cols-3 gap-4">
        {COLUMNS.map((col) => (
          <div
            key={col.status}
            role="group"
            aria-label={col.label}
            className="space-y-2 rounded border border-hairline p-2"
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => onDrop(e, col.status)}
          >
            <h2 className="text-xs font-semibold uppercase text-muted">{col.label}</h2>
            {tasks
              .filter((t) => t.status === col.status)
              .map((task) => (
                <div
                  key={task.id}
                  draggable
                  onDragStart={(e) => e.dataTransfer.setData('text/plain', String(task.id))}
                  className="space-y-1 rounded border border-hairline bg-card p-2 text-sm"
                >
                  <p>{task.title}</p>
                  {context && <p className="text-sm text-muted">{context(task)}</p>}
                  <Select
                    aria-label={`Move ${task.title}`}
                    value={task.status}
                    onChange={(e) => update.mutate({ task, status: e.target.value as TaskStatus })}
                  >
                    {COLUMNS.map((c) => (
                      <option key={c.status} value={c.status}>
                        {c.label}
                      </option>
                    ))}
                  </Select>
                </div>
              ))}
          </div>
        ))}
      </div>
    </div>
  );
}
