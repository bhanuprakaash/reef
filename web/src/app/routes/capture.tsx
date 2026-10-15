import { useEffect, useState } from 'react';
import { useCreateTask } from '@/modules/tasks/api/create-task';
import { useProjects } from '@/modules/projects/api/get-projects';
import { buttonClass, inputClass } from '@/utils/classes';

const KINDS = ['idea', 'todo', 'bug'] as const;
const LAST_PROJECT_KEY = 'reef.capture.lastProjectId';

export function CapturePage() {
  const projects = useProjects();
  const create = useCreateTask();
  const [projectId, setProjectId] = useState(() => localStorage.getItem(LAST_PROJECT_KEY) ?? '');
  const [kind, setKind] = useState<(typeof KINDS)[number]>('idea');
  const [text, setText] = useState('');

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') window.close();
    }
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, []);

  return (
    <div className="flex h-screen flex-col gap-2 bg-white p-4 text-sm dark:bg-neutral-900 dark:text-neutral-100">
      <form
        className="flex flex-col gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          localStorage.setItem(LAST_PROJECT_KEY, projectId);
          create.mutate(
            { projectId: Number(projectId), featureId: null, title: `${kind}: ${text}` },
            { onSuccess: () => window.close() },
          );
        }}
      >
        <select
          className={inputClass}
          aria-label="Capture project"
          value={projectId}
          onChange={(e) => setProjectId(e.target.value)}
          required
        >
          <option value="">Project</option>
          {projects.data?.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        <select
          className={inputClass}
          aria-label="Capture kind"
          value={kind}
          onChange={(e) => setKind(e.target.value as (typeof KINDS)[number])}
        >
          {KINDS.map((k) => (
            <option key={k} value={k}>
              {k}
            </option>
          ))}
        </select>
        <input
          className={inputClass}
          aria-label="Capture text"
          autoFocus
          value={text}
          onChange={(e) => setText(e.target.value)}
          required
        />
        <button type="submit" className={buttonClass} disabled={create.isPending}>
          Capture
        </button>
      </form>
    </div>
  );
}
