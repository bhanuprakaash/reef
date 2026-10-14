import { useSearchParams } from 'react-router';
import { useFeatures } from '@/modules/features/api/get-features';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useTasks } from '@/modules/tasks/api/get-tasks';
import { AddTaskForm } from '@/modules/tasks/components/add-task-form';
import { TaskBoard } from '@/modules/tasks/components/task-board';
import { TaskFilters } from '@/modules/tasks/components/task-filters';
import { TaskList } from '@/modules/tasks/components/task-list';
import type { Task } from '@/types/api';
import { buttonClass, errorClass } from '@/utils/classes';

export function TasksPage() {
  const [params, setParams] = useSearchParams();
  const project = params.get('project') ?? '';
  const feature = params.get('feature') ?? '';
  const status = params.get('status') ?? '';
  const view = params.get('view') === 'board' ? 'board' : 'list';

  const projects = useProjects();
  const features = useFeatures();
  const tasks = useTasks({
    project: project === '' ? undefined : Number(project),
    feature: feature === '' ? undefined : Number(feature),
  });

  const shown = tasks.data?.filter(
    (t) => status === '' || (status === 'done') === (t.status === 'done'),
  );

  function onFilter(name: 'project' | 'feature' | 'status', value: string) {
    const next = new URLSearchParams(params);
    if (value === '') next.delete(name);
    else next.set(name, value);
    if (name === 'project') next.delete('feature');
    setParams(next);
  }

  function onView(next: 'list' | 'board') {
    const nextParams = new URLSearchParams(params);
    if (next === 'list') nextParams.delete('view');
    else nextParams.set('view', next);
    setParams(nextParams);
  }

  function context(t: Task) {
    const p = projects.data?.find((x) => x.id === t.projectId);
    const f = features.data?.find((x) => x.id === t.featureId);
    return [p?.name, f?.name].filter(Boolean).join(' / ');
  }

  return (
    <div className="max-w-3xl space-y-6">
      <h1 className="text-xl font-semibold">Tasks</h1>
      <AddTaskForm projects={projects.data ?? []} features={features.data ?? []} />
      <div className="flex flex-wrap items-center justify-between gap-2">
        <TaskFilters
          projects={projects.data ?? []}
          features={features.data ?? []}
          project={project}
          feature={feature}
          status={status}
          onChange={onFilter}
        />
        <div className="flex gap-2">
          <button
            type="button"
            className={buttonClass}
            aria-pressed={view === 'list'}
            onClick={() => onView('list')}
          >
            List
          </button>
          <button
            type="button"
            className={buttonClass}
            aria-pressed={view === 'board'}
            onClick={() => onView('board')}
          >
            Board
          </button>
        </div>
      </div>
      {tasks.isError && <p className={errorClass}>{tasks.error.message}</p>}
      {view === 'board' ? (
        <TaskBoard tasks={shown ?? []} context={context} />
      ) : (
        <TaskList tasks={shown ?? []} context={context} />
      )}
    </div>
  );
}
