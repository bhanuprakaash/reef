import { useEffect, useState } from 'react';
import { useCurrentProject } from '@/lib/current-project';
import { useCreateTask } from '@/modules/tasks/api/create-task';
import { useProjects } from '@/modules/projects/api/get-projects';
import { Button } from '@/components/button';
import { Logo } from '@/components/logo';
import { SegmentedControl } from '@/components/segmented-control';

type Kind = 'idea' | 'todo' | 'bug';
const KINDS: { value: Kind; label: string }[] = [
  { value: 'idea', label: 'Idea' },
  { value: 'todo', label: 'Todo' },
  { value: 'bug', label: 'Bug' },
];

export function CapturePage() {
  const projects = useProjects();
  const create = useCreateTask();
  const [project, pickProject] = useCurrentProject(projects.data);
  const [kind, setKind] = useState<Kind>('todo');
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
    <form
      aria-label="Quick capture"
      className="flex h-screen flex-col gap-3 bg-main p-4 text-sm"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { projectId: Number(project?.id), featureId: null, title: `${kind}: ${text}` },
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
      <div className="flex items-center gap-2.5">
        <Logo size={22} />
        <input
          className="h-10 min-w-0 flex-1 bg-transparent text-xl text-fg outline-none"
          aria-label="Capture text"
          autoFocus
          value={text}
          onChange={(e) => setText(e.target.value)}
          required
        />
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <SegmentedControl label="Capture kind" options={KINDS} value={kind} onChange={setKind} />
        <div className="relative">
          <select
            className="h-[34px] appearance-none rounded-pill border border-line bg-transparent pr-7 pl-3 text-[13px] text-soft"
            aria-label="Capture project"
            value={project?.id ?? ''}
            onChange={(e) => {
              const picked = projects.data?.find((p) => String(p.id) === e.target.value);
              if (picked) pickProject(picked);
            }}
            required
          >
            <option value="">Project</option>
            {projects.data?.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-soft">
            ▾
          </span>
        </div>
        <span className="ml-auto text-xs text-muted">↵ save · esc close</span>
        <Button type="submit" variant="primary" loading={create.isPending}>
          Capture
        </Button>
      </div>
      {create.isError ? (
        <p className="text-[12.5px] text-danger">{create.error.message}</p>
      ) : create.isSuccess && !isDesktop ? (
        <p className="text-[12.5px] text-muted">Captured</p>
      ) : (
        <p className="text-[12.5px] text-muted">
          {project
            ? `Lands in Tasks as "${kind}: ${text}" on ${project.name}, no feature.`
            : 'Pick a project to capture into.'}
        </p>
      )}
    </form>
  );
}
