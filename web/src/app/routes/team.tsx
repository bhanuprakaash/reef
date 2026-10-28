import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate, useParams } from 'react-router';
import { buttonClass } from '@/components/button';
import { useSpaces } from '@/lib/api/get-spaces';
import { useTeams } from '@/lib/api/get-teams';
import { workItemsQuery } from '@/modules/work-items/api/get-work-items';
import { TeamQueue } from '@/modules/work-items/components/team-queue';
import { RoleRow } from '@/modules/teams/components/role-row';

export function TeamPage() {
  const { name = '' } = useParams();
  const navigate = useNavigate();
  const teams = useTeams();
  const items = useQuery({ ...workItemsQuery, refetchInterval: 5000 });
  const spaces = useSpaces();
  const team = teams.data?.find((t) => t.name === name);
  const error = teams.error ?? items.error ?? spaces.error;

  return (
    <div className="mx-auto flex w-full max-w-[820px] flex-col gap-6 py-6">
      {teams.isPending && <p className="text-sm text-muted">Loading...</p>}
      {error && <p className="text-sm text-danger">{error.message}</p>}
      {teams.data && !team && <p className="text-sm text-muted">No Team named {name}.</p>}
      {team && (
        <>
          <header className="flex flex-wrap items-start gap-3">
            <div className="min-w-0 flex-1 basis-60">
              <h1 className="text-[32px] font-normal tracking-[-0.02em]">{team.name}</h1>
              <p className="mt-1 text-[13px] text-muted">
                {team.responsibility || 'No responsibility set.'}
              </p>
            </div>
            <Link to={`/teams?team=${encodeURIComponent(team.name)}`} className={buttonClass()}>
              Edit team
            </Link>
          </header>
          <section aria-label="Roster" className="space-y-1.5">
            <h2 className="mb-2.5 text-[11px] uppercase tracking-[0.08em] text-muted">Roster</h2>
            {team.roles.map((role) => (
              <RoleRow
                key={role.name}
                role={role}
                selected={false}
                onSelect={() =>
                  navigate(
                    `/teams?team=${encodeURIComponent(team.name)}&role=${encodeURIComponent(role.name)}`,
                  )
                }
              />
            ))}
          </section>
          <TeamQueue
            items={(items.data ?? []).filter((i) => i.teamId === team.id)}
            spaces={spaces.data ?? []}
          />
        </>
      )}
    </div>
  );
}
