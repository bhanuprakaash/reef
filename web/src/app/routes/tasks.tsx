import { useSearchParams } from 'react-router';
import { useFeatures } from '@/modules/features/api/get-features';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useTasks } from '@/modules/tasks/api/get-tasks';
import { AddTaskForm } from '@/modules/tasks/components/add-task-form';
import { TaskBoard } from '@/modules/tasks/components/task-board';
import { TaskFilters } from '@/modules/tasks/components/task-filters';
import { TaskList } from '@/modules/tasks/components/task-list';
import { SegmentedControl } from '@/components/segmented-control';

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

  return (
    <div className="flex flex-col gap-5">
      <header className="flex flex-wrap items-center gap-3">
        <h1 className="text-[32px] font-normal tracking-[-0.02em]">Tasks</h1>
        <div className="ml-auto">
          <SegmentedControl
            label="View"
            options={[
              { value: 'list', label: 'List' },
              { value: 'board', label: 'Board' },
            ]}
            value={view}
            onChange={onView}
          />
        </div>
      </header>
      <AddTaskForm projects={projects.data ?? []} features={features.data ?? []} />
      <TaskFilters
        projects={projects.data ?? []}
        features={features.data ?? []}
        project={project}
        feature={feature}
        status={status}
        onChange={onFilter}
      />
      {tasks.isError && <p className="text-sm text-danger">{tasks.error.message}</p>}
      {view === 'board' ? (
        <TaskBoard tasks={shown ?? []} features={features.data ?? []} />
      ) : (
        <TaskList
          tasks={shown ?? []}
          projects={projects.data ?? []}
          features={features.data ?? []}
        />
      )}
    </div>
  );
}
