import { useUpdateTask } from '@/modules/tasks/api/update-task';
import type { Task } from '@/types/api';
import { buttonClass, errorClass, mutedClass } from '@/utils/classes';

type Props = {
  tasks: Task[];
  context?: (task: Task) => string;
};

export function TaskList({ tasks, context }: Props) {
  const update = useUpdateTask();

  if (tasks.length === 0) return <p className={mutedClass}>No tasks.</p>;

  return (
    <div className="space-y-2">
      {update.isError && <p className={errorClass}>{update.error.message}</p>}
      <ul className="divide-y divide-neutral-200 text-sm dark:divide-neutral-800">
        {tasks.map((task) => (
          <li key={task.id} className="flex items-center gap-3 py-2">
            <span
              className={`w-14 shrink-0 text-xs uppercase ${task.status === 'done' ? 'text-green-600' : 'text-neutral-500'}`}
            >
              {task.status}
            </span>
            <span className={`flex-1 ${task.status === 'done' ? 'line-through opacity-60' : ''}`}>
              {task.title}
            </span>
            {context && <span className={mutedClass}>{context(task)}</span>}
            {task.status !== 'done' && (
              <button
                type="button"
                className={buttonClass}
                disabled={update.isPending}
                onClick={() => update.mutate({ task, status: 'done' })}
              >
                Mark done
              </button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
