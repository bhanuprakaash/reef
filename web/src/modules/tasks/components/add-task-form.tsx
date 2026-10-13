import { useState } from 'react';
import { useCreateTask } from '@/modules/tasks/api/create-task';
import type { Feature, Project } from '@/types/api';
import { buttonClass, errorClass, inputClass } from '@/utils/classes';

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
      className="flex flex-wrap items-start gap-2"
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
        className={`${inputClass} w-64`}
        placeholder="New task"
        aria-label="Task title"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        required
      />
      <select
        className={inputClass}
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
        className={inputClass}
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
      <button type="submit" className={buttonClass} disabled={create.isPending}>
        Add task
      </button>
      {create.isError && <p className={errorClass}>{create.error.message}</p>}
    </form>
  );
}
