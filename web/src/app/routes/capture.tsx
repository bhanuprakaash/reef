import { useEffect, useState } from 'react';
import { useCreateTask } from '@/modules/tasks/api/create-task';
import { useProjects } from '@/modules/projects/api/get-projects';
import { Button } from '@/components/button';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';

const KINDS = ['idea', 'todo', 'bug'] as const;
const LAST_PROJECT_KEY = 'reef.capture.lastProjectId';

export function CapturePage() {
  const projects = useProjects();
  const create = useCreateTask();
  const [projectId, setProjectId] = useState(() => localStorage.getItem(LAST_PROJECT_KEY) ?? '');
  const [kind, setKind] = useState<(typeof KINDS)[number]>('idea');
  const [text, setText] = useState('');
  // A plain browser tab's window.close() does nothing, so show a status instead there;
  // the desktop capture window still closes on success.
  const isDesktop = typeof window !== 'undefined' && window.reef != null;

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') window.close();
    }
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, []);

  return (
    <div className="flex h-screen flex-col gap-2 bg-main p-4 text-sm text-fg">
      <form
        className="flex flex-col gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          localStorage.setItem(LAST_PROJECT_KEY, projectId);
          create.mutate(
            { projectId: Number(projectId), featureId: null, title: `${kind}: ${text}` },
            {
              onSuccess: () => {
                if (isDesktop) {
                  window.close();
                } else {
                  setText('');
                }
              },
            },
          );
        }}
      >
        <Select
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
        </Select>
        <Select
          aria-label="Capture kind"
          value={kind}
          onChange={(e) => setKind(e.target.value as (typeof KINDS)[number])}
        >
          {KINDS.map((k) => (
            <option key={k} value={k}>
              {k}
            </option>
          ))}
        </Select>
        <TextField
          aria-label="Capture text"
          autoFocus
          value={text}
          onChange={(e) => setText(e.target.value)}
          required
        />
        <Button variant="primary" type="submit" loading={create.isPending}>
          Capture
        </Button>
        {create.isError && <p className="text-sm text-danger">{create.error.message}</p>}
        {create.isSuccess && !isDesktop && <p className="text-sm text-muted">Captured</p>}
      </form>
    </div>
  );
}
