import { useInspector } from '@/modules/spaces/api/get-inspector';
import { deniedTools, isAllowed } from '@/modules/spaces/utils/denied-tools';
import type { Space, Role } from '@/types/api';
import { Creature } from '@/components/creature';

type Props = { space: Space; roles: Role[]; running: boolean };

export function DeniedToolRows({ space, roles }: Props) {
  const inspector = useInspector(space.id, true);
  const rows = deniedTools(inspector.data?.turns ?? []).flatMap((d) => {
    const role = roles.find((r) => r.botId === d.botId);
    return role ? [{ ...d, role }] : [];
  });

  if (rows.length === 0) return null;

  return (
    <div className="space-y-2">
      <ul className="space-y-2">
        {rows.map((d) => {
          const lead = d.role.lead;
          const backend = lead ? space.backend : d.role.backend;
          const added = isAllowed(d.role.tools, d.pattern);
          return (
            <li
              key={`${d.turnId}-${d.pattern}`}
              className="flex flex-wrap items-center gap-3 rounded-card border border-line bg-card px-4 py-3.5 [--creature-ring:var(--color-card)]"
            >
              <Creature look={d.role} size={24} />
              <p className="min-w-0 flex-1 basis-48">
                <span className="capitalize">{d.role.name}</span>
                {' was denied '}
                <code className="font-mono text-[13px]">{d.pattern}</code>
              </p>
              {backend === 'codex' ? (
                <span className="text-[13px] text-muted">Codex ignores allowed tools</span>
              ) : d.needsWrite && !d.role.write ? (
                <span className="text-[13px] text-muted">Needs write access</span>
              ) : added ? (
                <span className="text-[13px] text-done">Added</span>
              ) : (
                <span className="text-[13px] text-muted">Edit this Bot to allow the tool</span>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
