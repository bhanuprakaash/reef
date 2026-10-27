import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { useSearchParams } from 'react-router';
import { Button } from '@/components/button';
import { Dialog } from '@/components/dialog';
import { Select } from '@/components/select';
import { useTeams } from '@/lib/api/get-teams';
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
  const teams = useTeams();
  const projects = useProjects();
  const [current] = useCurrentProject(projects.data);
  const [params, setParams] = useSearchParams();
  const [adding, setAdding] = useState(false);

  const type = params.get('type') ?? 'all';
  const team = params.get('team') ?? 'all';
  const project = params.get('project') ?? String(current?.id ?? 'all');
  const filter = (key: string) => (value: string) =>
    setParams((p) => (p.set(key, value), p), { replace: true });

  const items = (workItems.data ?? []).filter(
    (i) =>
      (type === 'all' || i.type === type) &&
      (team === 'all' || String(i.teamId) === team) &&
      (project === 'all' || String(i.projectId) === project),
  );
  const addTo = projects.data?.find((p) => String(p.id) === project) ?? current;
  const error = workItems.error ?? teams.error ?? projects.error;

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
          aria-label="Team"
          value={team}
          onChange={filter('team')}
          options={[
            { value: 'all', label: 'All Teams' },
            ...(teams.data ?? []).map((t) => ({ value: String(t.id), label: t.name })),
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
          New work item
        </Button>
      </header>
      {error && <p className="text-sm text-danger">{error.message}</p>}
      <Board items={items} teams={teams.data ?? []} />
      <Dialog open={adding} onClose={() => setAdding(false)} title="New work item">
        {adding && addTo && (
          <AddWorkItemForm projectId={addTo.id} onClose={() => setAdding(false)} />
        )}
      </Dialog>
    </div>
  );
}
