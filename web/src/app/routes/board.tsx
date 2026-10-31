import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { useSearchParams } from 'react-router';
import { Button } from '@/components/button';
import { Dialog } from '@/components/dialog';
import { Select } from '@/components/select';
import { useBots } from '@/lib/api/get-bots';
import { useCurrentProject } from '@/lib/current-project';
import { workItemsQuery } from '@/modules/work-items/api/get-work-items';
import { AddWorkItemForm } from '@/modules/work-items/components/add-work-item-form';
import { Board } from '@/modules/work-items/components/board';
import { useProjects } from '@/modules/projects/api/get-projects';
import type { WorkItemType } from '@/types/api';

const TYPES: WorkItemType[] = [
  'feature',
  'bug',
  'incident',
  'investigation',
  'task',
  'improvement',
];

export function BoardPage() {
  const workItems = useQuery({ ...workItemsQuery, refetchInterval: 5000 });
  const bots = useBots();
  const projects = useProjects();
  const [current] = useCurrentProject(projects.data);
  const [params, setParams] = useSearchParams();
  const [adding, setAdding] = useState(false);

  const type = params.get('type') ?? 'all';
  const owner = params.get('owner') ?? 'all';
  const project = params.get('project') ?? String(current?.id ?? 'all');
  const filter = (key: string) => (value: string) =>
    setParams((p) => (p.set(key, value), p), { replace: true });

  const items = (workItems.data ?? []).filter(
    (i) =>
      (type === 'all' || i.type === type) &&
      (owner === 'all' || String(i.ownerBotId) === owner) &&
      (project === 'all' || String(i.projectId) === project),
  );
  const addTo = projects.data?.find((p) => String(p.id) === project) ?? current;
  const error = workItems.error ?? bots.error ?? projects.error;

  return (
    <div className="flex flex-col gap-4 py-2">
      <header className="flex flex-wrap items-center gap-2">
        <h1 className="mr-auto text-[32px] font-normal tracking-[-0.02em]">Board</h1>
        <Select
          variant="pill"
          aria-label="Type"
          value={type}
          onChange={filter('type')}
          options={[
            { value: 'all', label: 'All types' },
            ...TYPES.map((t) => ({ value: t, label: t })),
          ]}
        />
        <Select
          variant="pill"
          aria-label="Owner"
          value={owner}
          onChange={filter('owner')}
          options={[
            { value: 'all', label: 'All owners' },
            ...(bots.data ?? []).map((b) => ({ value: String(b.id), label: b.name })),
          ]}
        />
        <Select
          variant="pill"
          aria-label="Project"
          value={project}
          onChange={filter('project')}
          options={[
            { value: 'all', label: 'All projects' },
            ...(projects.data ?? []).map((p) => ({ value: String(p.id), label: p.name })),
          ]}
        />
        <Button type="button" variant="primary" disabled={!addTo} onClick={() => setAdding(true)}>
          New Work item
        </Button>
      </header>
      {error && <p className="text-sm text-danger">{error.message}</p>}
      <Board items={items} bots={bots.data ?? []} />
      <Dialog open={adding} onClose={() => setAdding(false)} title="New Work item">
        {adding && addTo && (
          <AddWorkItemForm projectId={addTo.id} onClose={() => setAdding(false)} />
        )}
      </Dialog>
    </div>
  );
}
