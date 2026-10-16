import { useState } from 'react';
import { useCreateTask } from '@/modules/tasks/api/create-task';
import type { Feature, Project } from '@/types/api';
import { Button } from '@/components/button';
import { Select } from '@/components/select';

type Props = {
  projects: Project[];
  features: Feature[];
  defaultProjectId: string;
};

export function AddTaskForm({ projects, features, defaultProjectId }: Props) {
  const [title, setTitle] = useState('');
  const [projectId, setProjectId] = useState(defaultProjectId);
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
        className="h-11 min-w-0 flex-1 basis-60 md:h-[38px] bg-transparent text-[15px] text-fg outline-none placeholder:text-muted"
        placeholder="New task"
        aria-label="Task title"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        required
      />
      <Select
        variant="pill"
        aria-label="Task project"
        value={projectId}
        onChange={(v) => {
          setProjectId(v);
          setFeatureId('');
        }}
        options={[
          { value: '', label: 'Project', disabled: true },
          ...projects.map((p) => ({ value: String(p.id), label: p.name })),
        ]}
      />
      <Select
        variant="pill"
        aria-label="Task feature"
        value={featureId}
        onChange={setFeatureId}
        options={[
          { value: '', label: 'No feature' },
          ...projectFeatures.map((f) => ({ value: String(f.id), label: f.name })),
        ]}
      />
      <Button
        variant="primary"
        type="submit"
        aria-label="Add task"
        disabled={projectId === ''}
        loading={create.isPending}
      >
        Add
      </Button>
      {create.isError && <p className="basis-full text-sm text-danger">{create.error.message}</p>}
    </form>
  );
}
