import type { Task } from '@/types/api';

export function TaskProgress({ tasks }: { tasks: Task[] }) {
  const done = tasks.filter((t) => t.status === 'done').length;
  return (
    <div>
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={tasks.length}
        aria-valuenow={done}
        className="h-1 overflow-hidden rounded-pill bg-track"
      >
        <div
          className="h-full rounded-pill bg-done"
          style={{ width: tasks.length === 0 ? 0 : `${(done / tasks.length) * 100}%` }}
        />
      </div>
      <div className="mt-2 text-xs text-muted">
        {tasks.length === 0 ? 'No fixes yet' : `${done} of ${tasks.length} done`}
      </div>
    </div>
  );
}
