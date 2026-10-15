import { useSearchParams } from 'react-router';
import { useTeams } from '@/lib/api/get-teams';
import { TeamEditor } from '@/modules/teams/components/team-editor';
import { TeamList } from '@/modules/teams/components/team-list';

export function TeamsPage() {
  const [params, setParams] = useSearchParams();
  const teams = useTeams();
  const name = params.get('team') || (teams.data?.[0]?.name ?? '');
  const team = teams.data?.find((t) => t.name === name);

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-[32px] font-normal tracking-[-0.02em]">Teams</h1>
        <p className="mt-1 text-[13px] text-muted">
          Define a team once, start any channel with it.
        </p>
      </header>
      {teams.isPending && <p className="text-sm text-muted">Loading...</p>}
      {teams.isError && <p className="text-sm text-danger">{teams.error.message}</p>}
      <div className="flex flex-wrap items-start gap-6">
        <TeamList
          teams={teams.data ?? []}
          selected={name}
          onSelect={(n) => setParams({ team: n })}
        />
        {team && <TeamEditor key={team.name} team={team} onSaved={(n) => setParams({ team: n })} />}
      </div>
    </div>
  );
}
