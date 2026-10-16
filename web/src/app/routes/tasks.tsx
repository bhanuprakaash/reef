import { useSearchParams } from 'react-router';
import { useFeatures } from '@/modules/features/api/get-features';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useTasks } from '@/modules/tasks/api/get-tasks';
import { AddTaskForm } from '@/modules/tasks/components/add-task-form';
import { TaskBoard } from '@/modules/tasks/components/task-board';
import { TaskFilters } from '@/modules/tasks/components/task-filters';
import { TaskList } from '@/modules/tasks/components/task-list';
import { useCurrentProject } from '@/lib/current-project';

export function TasksPage() {
  const [params, setParams] = useSearchParams();
  const feature = params.get('feature') ?? '';
  const status = params.get('status') ?? 'open';
  const view = params.get('view') === 'board' ? 'board' : 'list';

  const projects = useProjects();
  const features = useFeatures();
  const [current] = useCurrentProject(projects.data);
  const project = params.get('project') ?? (current ? String(current.id) : 'all');
  const defaultProjectId = project === 'all' ? String(current?.id ?? '') : project;
  const tasks = useTasks({
    project: project === 'all' ? undefined : Number(project),
    feature: feature === '' ? undefined : Number(feature),
  });

  const shown = tasks.data?.filter(
    (t) => status === 'all' || (status === 'done') === (t.status === 'done'),
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

  return (
    <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-5 py-6">
      <h1 className="text-[32px] font-normal tracking-[-0.02em]">Tasks</h1>
      <TaskFilters
        projects={projects.data ?? []}
        features={features.data ?? []}
        project={project}
        feature={feature}
        status={status}
        view={view}
        onChange={onFilter}
        onView={onView}
      />
      <AddTaskForm
        key={defaultProjectId}
        projects={projects.data ?? []}
        features={features.data ?? []}
        defaultProjectId={defaultProjectId}
      />
      {tasks.isError && <p className="text-sm text-danger">{tasks.error.message}</p>}
      {view === 'board' ? (
        <TaskBoard tasks={shown ?? []} features={features.data ?? []} />
      ) : (
        <TaskList
          tasks={shown ?? []}
          projects={projects.data ?? []}
          features={features.data ?? []}
          allProjects={project === 'all'}
        />
      )}
    </div>
  );
}
