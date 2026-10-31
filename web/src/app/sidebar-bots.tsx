import { useQuery } from '@tanstack/react-query';
import { ResourceRow } from '@/app/resource-row';
import { byRecent } from '@/app/sidebar-spaces';
import { useBots } from '@/lib/api/get-bots';
import { spacesQuery } from '@/lib/api/get-spaces';
import type { Project } from '@/types/api';

const RECENT_BOTS = 5;

export function SidebarBots({ project }: { project?: Project }) {
  const spaces = useQuery({ ...spacesQuery(project?.id), refetchInterval: 5000 });
  const bots = useBots().data ?? [];
  const recent = [
    ...new Set(
      (spaces.data ?? [])
        .filter((c) => c.kind === 'conversation')
        .sort(byRecent)
        .map((c) => c.botId),
    ),
  ].flatMap((id) => bots.find((b) => b.id === id) ?? []);
  if (recent.length === 0) return null;
  return (
    <section aria-label="Bots" className="flex flex-col gap-0.5">
      <h4 className="px-2.5 pt-3.5 pb-1 text-xs text-muted">Bots</h4>
      <ul className="flex flex-col gap-0.5">
        {recent.slice(0, RECENT_BOTS).map((b) => (
          <li key={b.id}>
            <ResourceRow resource={{ kind: 'bot', id: b.id, title: b.name }} />
          </li>
        ))}
      </ul>
    </section>
  );
}
