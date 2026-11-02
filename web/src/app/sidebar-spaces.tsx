import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { SectionHeading } from '@/app/section-heading';
import { SpaceRow } from '@/app/space-row';
import { Dialog } from '@/components/dialog';
import { spacesQuery } from '@/lib/api/get-spaces';
import { pinKindOf, usePins } from '@/lib/api/pins';
import { NewConversationForm } from '@/modules/spaces/components/new-conversation-form';
import { NewSpaceForm } from '@/modules/spaces/components/new-space-form';
import { workItemsQuery } from '@/modules/work-items/api/get-work-items';
import type { Project, SpacePreview } from '@/types/api';

const RECENT_CONVERSATIONS = 8;

export const byRecent = (a: SpacePreview, b: SpacePreview) =>
  (b.lastMessage?.createdAt ?? b.createdAt).localeCompare(a.lastMessage?.createdAt ?? a.createdAt);

function Section({
  to,
  label,
  addLabel,
  onAdd,
  empty,
  spaces,
}: {
  to: string;
  label: string;
  addLabel: string;
  onAdd: () => void;
  empty: string;
  spaces: SpacePreview[];
}) {
  return (
    <>
      <SectionHeading to={to} label={label} addLabel={addLabel} onAdd={onAdd} />
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

export function SidebarSpaces({ project }: { project?: Project }) {
  const spaces = useQuery({ ...spacesQuery(project?.id), refetchInterval: 5000 });
  const items = useQuery({ ...workItemsQuery, refetchInterval: 5000 });
  const pins = usePins().data ?? [];
  const [creating, setCreating] = useState<'space' | 'conversation' | null>(null);

  const mine = spaces.data ?? [];
  const handedOver = (c: SpacePreview) => {
    const status = items.data?.find((f) => f.id === c.workItemId)?.status;
    return status === 'review' || status === 'done' || status === 'dropped';
  };
  const pinnedFirst = (a: SpacePreview, b: SpacePreview) => {
    const isPinned = (c: SpacePreview) =>
      pins.some((p) => p.kind === pinKindOf(c) && p.id === c.id);
    return Number(isPinned(b)) - Number(isPinned(a)) || byRecent(a, b);
  };
  const live = mine
    .filter(
      (c) =>
        c.kind !== 'conversation' &&
        c.kind !== 'consultation' &&
        c.closedAt === null &&
        !handedOver(c),
    )
    .sort(pinnedFirst);
  const conversations = mine.filter((c) => c.kind === 'conversation').sort(pinnedFirst);

  return (
    <section aria-label={project?.name ?? 'All projects'} className="flex flex-col gap-0.5">
      <Section
        to="/spaces"
        label="Spaces"
        addLabel="New Space"
        onAdd={() => setCreating('space')}
        empty="No Spaces open"
        spaces={live}
      />
      <Section
        to="/conversations"
        label="Conversations"
        addLabel="New conversation"
        onAdd={() => setCreating('conversation')}
        empty="No conversations yet"
        spaces={conversations.slice(0, RECENT_CONVERSATIONS)}
      />
      <Dialog
        open={creating !== null}
        onClose={() => setCreating(null)}
        title={creating === 'conversation' ? 'New conversation' : 'New Space'}
      >
        {creating === 'space' && (
          <NewSpaceForm
            projectId={project?.id}
            workItemId={null}
            onClose={() => setCreating(null)}
          />
        )}
        {creating === 'conversation' && (
          <NewConversationForm projectId={project?.id} onClose={() => setCreating(null)} />
        )}
      </Dialog>
    </section>
  );
}
