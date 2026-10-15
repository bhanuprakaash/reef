import { useUpdateTask } from '@/modules/tasks/api/update-task';
import type { Task } from '@/types/api';
import { Button } from '@/components/button';

type Props = {
  tasks: Task[];
  context?: (task: Task) => string;
};

export function TaskList({ tasks, context }: Props) {
  const update = useUpdateTask();

  if (tasks.length === 0) return <p className="text-sm text-muted">No tasks.</p>;

  return (
    <div className="space-y-2">
      {update.isError && <p className="text-sm text-danger">{update.error.message}</p>}
      <ul className="divide-y divide-hairline text-sm">
        {tasks.map((task) => (
          <li key={task.id} className="flex items-center gap-3 py-2">
            <span
              className={`w-14 shrink-0 text-xs uppercase ${task.status === 'done' ? 'text-done' : 'text-muted'}`}
            >
              {task.status}
            </span>
            <span className={`flex-1 ${task.status === 'done' ? 'line-through opacity-60' : ''}`}>
              {task.title}
            </span>
            {context && <span className="text-sm text-muted">{context(task)}</span>}
            {task.status !== 'done' && (
              <Button
                type="button"
                disabled={update.isPending}
                onClick={() => update.mutate({ task, status: 'done' })}
              >
                Mark done
              </Button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
