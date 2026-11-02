import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { byRecent } from '@/app/sidebar-spaces';
import { SpaceRow } from '@/app/space-row';
import { EmptyState } from '@/components/empty-state';
import { TextField } from '@/components/text-field';
import { spacesQuery } from '@/lib/api/get-spaces';
import { pinKindOf, usePins } from '@/lib/api/pins';
import { useCurrentProject } from '@/lib/current-project';
import { useProjects } from '@/modules/projects/api/get-projects';
import { dateGroup } from '@/utils/date-group';

const GROUPS = ['Pinned', 'Today', 'Yesterday', 'Last 7 days', 'Older'] as const;

function SpaceList({ title, conversations }: { title: string; conversations: boolean }) {
  const projects = useProjects();
  const [project] = useCurrentProject(projects.data);
  const spaces = useQuery({ ...spacesQuery(project?.id), refetchInterval: 5000 });
  const pins = usePins().data ?? [];
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
  const groupOf = (c: (typeof rows)[number]) =>
    pins.some((p) => p.kind === pinKindOf(c) && p.id === c.id)
      ? 'Pinned'
      : dateGroup(c.lastMessage?.createdAt ?? c.createdAt);
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
      {GROUPS.map((group) => {
        const inGroup = rows.filter((c) => groupOf(c) === group);
        if (inGroup.length === 0) return null;
        return (
          <section key={group} aria-label={group} className="flex flex-col gap-0.5">
            <h2 className="px-2.5 pb-1 text-xs text-muted">{group}</h2>
            <ul className="flex flex-col gap-0.5">
              {inGroup.map((c) => (
                <li key={c.id}>
                  <SpaceRow space={c} />
                </li>
              ))}
            </ul>
          </section>
        );
      })}
    </div>
  );
}

export const SpacesPage = () => <SpaceList title="Spaces" conversations={false} />;

export const ConversationsPage = () => <SpaceList title="Conversations" conversations />;
