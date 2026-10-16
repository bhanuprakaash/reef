import { Link } from 'react-router';
import { useUpdateTask } from '@/modules/tasks/api/update-task';
import type { Feature, Project, Task } from '@/types/api';
import { EmptyState } from '@/components/empty-state';

type Props = {
  tasks: Task[];
  projects?: Project[];
  features?: Feature[];
  compact?: boolean;
  allProjects?: boolean;
};

export function TaskList({ tasks, projects = [], features, compact, allProjects }: Props) {
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

  const rank = { doing: 0, todo: 1, done: 2 };
  const groups = new Map<string, Task[]>();
  for (const task of [...tasks].sort((a, b) => rank[a.status] - rank[b.status])) {
    const key = task.featureId === null ? `p${task.projectId}` : `f${task.featureId}`;
    groups.set(key, [...(groups.get(key) ?? []), task]);
  }

  function groupName(task: Task) {
    if (task.featureId !== null) return features?.find((f) => f.id === task.featureId)?.name;
    if (!allProjects) return 'No feature';
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
                {groupName(group[0])}{' '}
                <span className="text-faint">
                  {group.filter((t) => t.status !== 'done').length}
                </span>
              </h2>
            )}
            <ul>
              {group.map((task) => {
                const done = task.status === 'done';
                return (
                  <li key={task.id} className={compact ? '' : 'border-t border-hairline'}>
                    <div
                      className={`flex items-center ${compact ? 'min-h-9 gap-2.5 text-[13px]' : 'min-h-11 gap-3.5 px-[18px]'} ${done ? 'text-faint' : ''}`}
                    >
                      <input
                        type="checkbox"
                        aria-label={task.title}
                        className={`accent-pill ${compact ? 'size-4' : 'size-[18px]'}`}
                        checked={done}
                        disabled={update.isPending}
                        onChange={() => update.mutate({ task, status: done ? 'todo' : 'done' })}
                      />
                      <Link
                        to={`/tasks/${task.id}`}
                        className="flex min-w-0 flex-1 gap-1.5 py-1.5 hover:underline"
                      >
                        <span className="shrink-0 text-muted">#{task.id}</span>
                        <span className={`truncate ${done ? 'line-through' : ''}`}>
                          {task.title}
                        </span>
                      </Link>
                      {task.status === 'doing' && (
                        <span className="flex h-[22px] items-center rounded-pill bg-working/15 px-[9px] text-xs text-working">
                          doing
                        </span>
                      )}
                    </div>
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
