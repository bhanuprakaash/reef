import { useState } from 'react';
import { useCreateTask } from '@/modules/tasks/api/create-task';
import type { Feature, Project } from '@/types/api';
import { Button } from '@/components/button';

const selectClass =
  'h-8 rounded-pill border border-line bg-transparent px-3 text-[13px] text-soft outline-none focus-visible:ring-3 focus-visible:ring-ring';

type Props = {
  projects: Project[];
  features: Feature[];
};

export function AddTaskForm({ projects, features }: Props) {
  const [title, setTitle] = useState('');
  const [projectId, setProjectId] = useState('');
  const [featureId, setFeatureId] = useState('');
  const create = useCreateTask();
  const projectFeatures = features.filter((f) => String(f.projectId) === projectId);

  return (
    <form
      className="flex flex-wrap items-center gap-2 rounded-2xl border border-line bg-field py-1.5 pr-1.5 pl-4"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          {
            projectId: Number(projectId),
            featureId: featureId === '' ? null : Number(featureId),
            title,
          },
          { onSuccess: () => setTitle('') },
        );
      }}
    >
      <input
        className="h-[38px] min-w-0 flex-1 basis-60 bg-transparent text-[15px] text-fg outline-none placeholder:text-muted"
        placeholder="New task"
        aria-label="Task title"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        required
      />
      <select
        className={selectClass}
        aria-label="Task project"
        value={projectId}
        onChange={(e) => {
          setProjectId(e.target.value);
          setFeatureId('');
        }}
        required
      >
        <option value="">Project</option>
        {projects.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </select>
      <select
        className={selectClass}
        aria-label="Task feature"
        value={featureId}
        onChange={(e) => setFeatureId(e.target.value)}
      >
        <option value="">No feature</option>
        {projectFeatures.map((f) => (
          <option key={f.id} value={f.id}>
            {f.name}
          </option>
        ))}
      </select>
      <Button variant="primary" type="submit" aria-label="Add task" loading={create.isPending}>
        Add
      </Button>
      {create.isError && <p className="basis-full text-sm text-danger">{create.error.message}</p>}
    </form>
  );
}
