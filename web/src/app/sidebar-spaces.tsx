import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { SpaceRow } from '@/app/space-row';
import { spacesQuery } from '@/lib/api/get-spaces';
import { workItemsQuery } from '@/modules/work-items/api/get-work-items';
import type { Project, SpacePreview } from '@/types/api';

const RECENT_CONVERSATIONS = 8;

export const byRecent = (a: SpacePreview, b: SpacePreview) =>
  (b.lastMessage?.createdAt ?? b.createdAt).localeCompare(a.lastMessage?.createdAt ?? a.createdAt);

function Section({
  to,
  label,
  empty,
  spaces,
}: {
  to: string;
  label: string;
  empty: string;
  spaces: SpacePreview[];
}) {
  return (
    <>
      <h4 className="px-2.5 pt-3.5 pb-1 text-xs text-muted">
        <Link to={to} className="hover:text-fg">
          {label}
        </Link>
      </h4>
      {spaces.length === 0 && <p className="px-2.5 pb-1 text-[13px] text-faint">{empty}</p>}
      <ul className="flex flex-col gap-0.5">
        {spaces.map((c) => (
          <li key={c.id}>
            <SpaceRow space={c} />
          </li>
        ))}
      </ul>
    </>
  );
}

export function SidebarSpaces({ project, query }: { project: Project; query: string }) {
  const spaces = useQuery({ ...spacesQuery, refetchInterval: 5000 });
  const items = useQuery({ ...workItemsQuery, refetchInterval: 5000 });

  const q = query.trim().toLowerCase();
  const mine = (spaces.data ?? []).filter(
    (c) => c.projectId === project.id && c.name.toLowerCase().includes(q),
  );
  const handedOver = (c: SpacePreview) => {
    const status = items.data?.find((f) => f.id === c.workItemId)?.status;
    return status === 'review' || status === 'done' || status === 'dropped';
  };
  const live = mine
    .filter((c) => c.kind !== 'conversation' && c.closedAt === null && !handedOver(c))
    .sort(byRecent);
  const conversations = mine.filter((c) => c.kind === 'conversation').sort(byRecent);

  return (
    <section aria-label={project.name} className="flex flex-col gap-0.5">
      <Section to="/spaces" label="Spaces" empty="No Spaces open" spaces={live} />
      <Section
        to="/conversations"
        label="Conversations"
        empty="No conversations yet"
        spaces={q === '' ? conversations.slice(0, RECENT_CONVERSATIONS) : conversations}
      />
    </section>
  );
}
