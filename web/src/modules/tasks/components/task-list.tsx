import { useUpdateTask } from '@/modules/tasks/api/update-task';
import type { Feature, Project, Task } from '@/types/api';
import { EmptyState } from '@/components/empty-state';

type Props = {
  tasks: Task[];
  projects?: Project[];
  features?: Feature[];
  compact?: boolean;
};

export function TaskList({ tasks, projects = [], features, compact }: Props) {
  const update = useUpdateTask();

  if (tasks.length === 0) {
    return (
      <EmptyState
        title="No tasks yet"
        body={
          features && window.reef != null ? 'Drop an idea, todo or bug with ⌘⇧Space.' : undefined
        }
        role="developer"
        compact={compact}
      />
    );
  }

  const groups = new Map<string, Task[]>();
  for (const task of tasks) {
    const key = task.featureId === null ? `p${task.projectId}` : `f${task.featureId}`;
    groups.set(key, [...(groups.get(key) ?? []), task]);
  }

  function groupName(task: Task) {
    if (task.featureId !== null) return features?.find((f) => f.id === task.featureId)?.name;
    const project = projects.find((p) => p.id === task.projectId)?.name;
    return `${project ?? ''} · no feature`;
  }

  return (
    <div className="space-y-2">
      {update.isError && <p className="text-sm text-danger">{update.error.message}</p>}
      <div
        className={
          compact ? '' : 'divide-y divide-hairline overflow-hidden rounded-card bg-sidebar'
        }
      >
        {[...groups.entries()].map(([key, group]) => (
          <section key={key}>
            {features && (
              <h2 className="px-[18px] pt-3.5 pb-1.5 text-[12.5px] font-normal text-muted">
                {groupName(group[0])}
              </h2>
            )}
            <ul>
              {group.map((task) => {
                const done = task.status === 'done';
                return (
                  <li key={task.id} className={compact ? '' : 'border-t border-hairline'}>
                    <label
                      className={`flex items-center ${compact ? 'min-h-9 gap-2.5 text-[13px]' : 'min-h-[52px] gap-3.5 px-[18px]'} ${done ? 'text-faint' : ''}`}
                    >
                      <input
                        type="checkbox"
                        className={`accent-pill ${compact ? 'size-4' : 'size-[18px]'}`}
                        checked={done}
                        disabled={done || update.isPending}
                        onChange={() => update.mutate({ task, status: 'done' })}
                      />
                      <span className={`flex-1 ${done ? 'line-through' : ''}`}>{task.title}</span>
                      {!done && (
                        <span
                          className={`flex h-[22px] items-center rounded-pill px-[9px] text-xs ${task.status === 'doing' ? 'bg-working/15 text-working' : 'bg-bubble text-soft'}`}
                        >
                          {task.status}
                        </span>
                      )}
                    </label>
                  </li>
                );
              })}
            </ul>
          </section>
        ))}
      </div>
    </div>
  );
}
