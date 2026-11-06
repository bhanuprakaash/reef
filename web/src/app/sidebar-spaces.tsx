import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { SidebarSection } from '@/app/sidebar-section';
import { SpaceRow } from '@/app/space-row';
import { Dialog } from '@/components/dialog';
import { spacesQuery } from '@/lib/api/get-spaces';
import { pinKindOf, usePins } from '@/lib/api/pins';
import { NewConversationForm } from '@/modules/spaces/components/new-conversation-form';
import { NewSpaceForm } from '@/modules/spaces/components/new-space-form';
import { workItemsQuery } from '@/modules/work-items/api/get-work-items';
import type { Project, SpacePreview } from '@/types/api';

export const byRecent = (a: SpacePreview, b: SpacePreview) =>
  (b.lastMessage?.createdAt ?? b.createdAt).localeCompare(a.lastMessage?.createdAt ?? a.createdAt);

export function SidebarSpaces({ project }: { project?: Project }) {
  const spaces = useQuery({ ...spacesQuery(project?.id), refetchInterval: 5000 });
  const everything = useQuery({ ...spacesQuery(), refetchInterval: 5000 });
  const items = useQuery({ ...workItemsQuery, refetchInterval: 5000 });
  const pins = usePins().data ?? [];
  const [creating, setCreating] = useState<'space' | 'conversation' | null>(null);

  const handedOver = (c: SpacePreview) => {
    const status = items.data?.find((f) => f.id === c.workItemId)?.status;
    return status === 'review' || status === 'done' || status === 'dropped';
  };
  const isPinned = (c: SpacePreview) => pins.some((p) => p.kind === pinKindOf(c) && p.id === c.id);
  const mine = spaces.data ?? [];
  const pool = [
    ...mine,
    ...(everything.data ?? []).filter((c) => isPinned(c) && !mine.some((m) => m.id === c.id)),
  ];
  const live = pool
    .filter(
      (c) =>
        c.kind !== 'conversation' &&
        (isPinned(c) || (c.kind !== 'consultation' && c.closedAt === null && !handedOver(c))),
    )
    .sort(byRecent);
  const conversations = pool.filter((c) => c.kind === 'conversation').sort(byRecent);

  return (
    <section aria-label={project?.name ?? 'All projects'} className="flex flex-col gap-0.5">
      <SidebarSection
        id="spaces"
        to="/spaces"
        label="Spaces"
        addLabel="New Space"
        onAdd={() => setCreating('space')}
        empty="No Spaces open"
        items={live}
        isPinned={isPinned}
        row={(c) => <SpaceRow space={c} />}
      />
      <SidebarSection
        id="conversations"
        to="/conversations"
        label="Conversations"
        addLabel="New conversation"
        onAdd={() => setCreating('conversation')}
        empty="No conversations yet"
        items={conversations}
        isPinned={isPinned}
        row={(c) => <SpaceRow space={c} />}
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
