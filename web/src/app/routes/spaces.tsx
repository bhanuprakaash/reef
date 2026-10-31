import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { byRecent } from '@/app/sidebar-spaces';
import { SpaceRow } from '@/app/space-row';
import { EmptyState } from '@/components/empty-state';
import { TextField } from '@/components/text-field';
import { spacesQuery } from '@/lib/api/get-spaces';
import { useCurrentProject } from '@/lib/current-project';
import { useProjects } from '@/modules/projects/api/get-projects';

function SpaceList({ title, conversations }: { title: string; conversations: boolean }) {
  const projects = useProjects();
  const [project] = useCurrentProject(projects.data);
  const spaces = useQuery({ ...spacesQuery(project?.id), refetchInterval: 5000 });
  const [query, setQuery] = useState('');

  const q = query.trim().toLowerCase();
  const rows = (spaces.data ?? [])
    .filter(
      (c) =>
        c.kind !== 'consultation' &&
        (c.kind === 'conversation') === conversations &&
        c.name.toLowerCase().includes(q),
    )
    .sort(byRecent);
  const error = projects.error ?? spaces.error;

  return (
    <div className="mx-auto flex w-full max-w-[820px] flex-col gap-4 py-6">
      <h1 className="text-[32px] font-normal tracking-[-0.02em]">{title}</h1>
      <TextField
        type="search"
        aria-label="Search"
        placeholder="Search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {error && <p className="text-sm text-danger">{error.message}</p>}
      {spaces.data && rows.length === 0 && (
        <EmptyState
          title={`No ${title.toLowerCase()}`}
          body="Start one from the + menu."
          face="jellyfish"
        />
      )}
      <ul className="flex flex-col gap-0.5">
        {rows.map((c) => (
          <li key={c.id}>
            <SpaceRow space={c} />
          </li>
        ))}
      </ul>
    </div>
  );
}

export const SpacesPage = () => <SpaceList title="Spaces" conversations={false} />;

export const ConversationsPage = () => <SpaceList title="Conversations" conversations />;
