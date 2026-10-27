import { useEffect, useState } from 'react';
import { useCurrentProject } from '@/lib/current-project';
import { useTeammates } from '@/lib/api/get-teammates';
import { useCreateWorkItem } from '@/modules/work-items/api/create-work-item';
import { useProjects } from '@/modules/projects/api/get-projects';
import { useStartConversation } from '@/modules/spaces/api/start-conversation';
import { Button } from '@/components/button';
import { Logo } from '@/components/logo';
import { Select } from '@/components/select';
import { TextArea } from '@/components/text-area';
import { Toggle } from '@/components/toggle';
import type { WorkItemType } from '@/types/api';

const TYPES: { value: WorkItemType; label: string }[] = [
  { value: 'task', label: 'Task' },
  { value: 'feature', label: 'Feature' },
  { value: 'bug', label: 'Bug' },
  { value: 'incident', label: 'Incident' },
  { value: 'investigation', label: 'Investigation' },
  { value: 'improvement', label: 'Improvement' },
];

export function CapturePage() {
  const projects = useProjects();
  const teammates = useTeammates();
  const createItem = useCreateWorkItem();
  const startConversation = useStartConversation();
  const [project, pickProject] = useCurrentProject(projects.data);
  const [type, setType] = useState<WorkItemType>('task');
  const [text, setText] = useState('');
  const [description, setDescription] = useState('');
  const [talk, setTalk] = useState(false);
  const [teammateId, setTeammateId] = useState('');
  const teammate = teammates.data?.find((t) => String(t.id) === teammateId) ?? teammates.data?.[0];
  const create = talk ? startConversation : createItem;
  // A plain browser tab's window.close() does nothing, so show a status instead there;
  // the desktop capture window still closes on success.
  const isDesktop = typeof window !== 'undefined' && window.reef != null;

  useEffect(() => {
    const els = [document.documentElement, document.body];
    els.forEach((el) => (el.style.background = 'transparent'));
    return () => els.forEach((el) => (el.style.background = ''));
  }, []);

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
      className="app-drag flex flex-col gap-3 rounded-[22px] border border-line bg-main/92 p-4 text-sm"
      onSubmit={(e) => {
        e.preventDefault();
        const options = {
          onSuccess: () => {
            if (isDesktop) {
              window.close();
            } else {
              setText('');
              setDescription('');
            }
          },
        };
        if (talk && teammate) {
          const time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
          startConversation.mutate(
            {
              projectId: Number(project?.id),
              teammateId: teammate.id,
              name: `${teammate.name} ${time}`,
              text,
            },
            options,
          );
        } else {
          createItem.mutate(
            { projectId: Number(project?.id), type, title: text, description },
            options,
          );
        }
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
      {!talk && (
        <TextArea
          className="w-full bg-transparent"
          rows={2}
          aria-label="Capture description"
          placeholder="Description"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      )}
      <div className="flex flex-wrap items-center gap-2">
        {talk ? (
          <Select
            variant="pill"
            className="max-w-36 min-w-0 [&>button]:h-[34px]!"
            aria-label="Capture teammate"
            value={teammate ? String(teammate.id) : ''}
            onChange={setTeammateId}
            options={(teammates.data ?? []).map((t) => ({ value: String(t.id), label: t.name }))}
          />
        ) : (
          <Select
            variant="pill"
            className="max-w-36 min-w-0 [&>button]:h-[34px]!"
            aria-label="Capture type"
            value={type}
            onChange={(v) => setType(v as WorkItemType)}
            options={TYPES}
          />
        )}
        <Select
          variant="pill"
          className="max-w-44 min-w-0 [&>button]:h-[34px]!"
          aria-label="Capture project"
          value={project ? String(project.id) : ''}
          onChange={(id) => {
            const picked = projects.data?.find((p) => String(p.id) === id);
            if (picked) pickProject(picked);
          }}
          options={[
            { value: '', label: 'Project', disabled: true },
            ...(projects.data ?? []).map((p) => ({ value: String(p.id), label: p.name })),
          ]}
        />
        <label className="flex items-center gap-2 text-[13px] text-soft">
          <Toggle checked={talk} onChange={setTalk} label="Talk to a Teammate" />
          Talk to a Teammate
        </label>
        <span className="ml-auto text-xs text-muted">↵ save · esc close</span>
        <Button
          type="submit"
          variant="primary"
          className="h-[34px]! text-[13.5px]!"
          disabled={!project || (talk && !teammate)}
          loading={create.isPending}
        >
          Capture
        </Button>
      </div>
      {create.isError ? (
        <p className="text-[12.5px] text-danger">{create.error.message}</p>
      ) : create.isSuccess && !isDesktop ? (
        <p className="text-[12.5px] text-muted">Captured</p>
      ) : (
        <p className="text-[12.5px] text-muted">
          {!project
            ? 'Pick a project to capture into.'
            : talk
              ? `Starts a conversation with ${teammate?.name ?? 'a Teammate'} in ${project.name}.`
              : `Lands in the ${project.name} Backlog as a ${type} titled "${text}".`}
        </p>
      )}
    </form>
  );
}
