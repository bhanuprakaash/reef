import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';
import { ResourceRow } from '@/app/resource-row';
import { SectionHeading } from '@/app/section-heading';
import { byRecent } from '@/app/sidebar-spaces';
import { RowMenu } from '@/components/row-menu';
import { useBots } from '@/lib/api/get-bots';
import { spacesQuery } from '@/lib/api/get-spaces';
import { usePins, useTogglePin } from '@/lib/api/pins';
import { NewBotDialog } from '@/modules/teams/components/new-bot-dialog';
import type { Project } from '@/types/api';

const RECENT_BOTS = 5;

function BotItem({ id, name }: { id: number; name: string }) {
  const { pinned, toggle } = useTogglePin('bot', id);
  return (
    <RowMenu items={[{ label: pinned ? 'Unpin' : 'Pin', onClick: () => toggle.mutate() }]}>
      <ResourceRow resource={{ kind: 'bot', id, title: name }} />
    </RowMenu>
  );
}

export function SidebarBots({ project }: { project?: Project }) {
  const spaces = useQuery({ ...spacesQuery(project?.id), refetchInterval: 5000 });
  const bots = useBots().data ?? [];
  const pinnedIds = (usePins().data ?? []).flatMap((p) => (p.kind === 'bot' ? p.id : []));
  const [creating, setCreating] = useState(false);
  const navigate = useNavigate();
  const find = (id: number | null) => bots.find((b) => b.id === id) ?? [];
  const recent = [
    ...new Set(
      (spaces.data ?? [])
        .filter((c) => c.kind === 'conversation')
        .sort(byRecent)
        .map((c) => c.botId),
    ),
  ]
    .flatMap(find)
    .slice(0, RECENT_BOTS);
  const listed = [...new Set([...recent, ...pinnedIds.flatMap(find)])].sort(
    (a, b) => Number(pinnedIds.includes(b.id)) - Number(pinnedIds.includes(a.id)),
  );
  return (
    <section aria-label="Bots" className="flex flex-col gap-0.5">
      <SectionHeading to="/bots" label="Bots" addLabel="New Bot" onAdd={() => setCreating(true)} />
      <ul className="flex flex-col gap-0.5">
        {listed.map((b) => (
          <li key={b.id}>
            <BotItem id={b.id} name={b.name} />
          </li>
        ))}
      </ul>
      <NewBotDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={(id) => void navigate(`/bots?bot=${id}`)}
      />
    </section>
  );
}
