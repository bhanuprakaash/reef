import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { useTeams } from '@/lib/api/get-teams';
import { useRunningTurns } from '@/modules/spaces/api/get-turns';
import { backendLabel } from '@/modules/teams/utils/backend-label';
import type { Role } from '@/types/api';
import { Creature } from '@/components/creature';
import { MentionCards } from '@/components/markdown';
import { StatusDot } from '@/components/status-dot';
import { claudeModels } from '@/utils/models';

function modelLabel(role: Role) {
  if (!role.model) return 'default model';
  const known = role.backend === 'claude' && claudeModels.find((o) => o.value === role.model);
  return known ? known.label : role.model;
}

export function MentionCard({
  team,
  role,
  working,
}: {
  team: string;
  role: Role;
  working: boolean;
}) {
  const description = role.description.split('\n')[0];
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2.5">
        <Creature look={role} state={working ? 'working' : 'idle'} size={36} animate={working} />
        <div className="min-w-0 flex-1">
          <div className="truncate capitalize">{role.name}</div>
          <div className="truncate text-[13px] text-soft">
            {backendLabel(role.backend)} · {modelLabel(role)}
          </div>
        </div>
        <Link
          to={`/teams?${new URLSearchParams({ team, role: role.name })}`}
          className="rounded-sm text-[13px] text-soft outline-none hover:text-fg focus-visible:ring-2 focus-visible:ring-ring"
        >
          Edit
        </Link>
      </div>
      {description && <p className="truncate text-[13px] text-soft">{description}</p>}
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-[13px]">
        <dt className="text-muted">State</dt>
        <dd className="flex items-center gap-2">
          <StatusDot status={working ? 'working' : 'asleep'} />
          {working ? 'Working' : 'Idle'}
        </dd>
        <dt className="text-muted">Access</dt>
        <dd>{role.write ? 'Can write' : 'Read-only'}</dd>
        {role.backend === 'claude' && (
          <>
            <dt className="text-muted">Tools</dt>
            <dd>{role.tools.length}</dd>
          </>
        )}
      </dl>
    </div>
  );
}

export function MentionCardsProvider({
  spaceId,
  team,
  children,
}: {
  spaceId: number;
  team: string;
  children: ReactNode;
}) {
  const teams = useTeams();
  const running = useRunningTurns();
  const found = teams.data?.find((t) => t.name === team);
  const working = (running.data ?? []).filter((t) => t.spaceId === spaceId).map((t) => t.role);

  function card(name: string) {
    const role = found?.roles.find((r) => r.name === name);
    if (!found || !role) return null;
    return <MentionCard team={team} role={role} working={working.includes(name)} />;
  }

  return <MentionCards.Provider value={card}>{children}</MentionCards.Provider>;
}
