import { useSearchParams } from 'react-router';
import { useTeams } from '@/lib/api/get-teams';
import { TeamEditor } from '@/modules/teams/components/team-editor';
import { TeamList } from '@/modules/teams/components/team-list';
import { errorClass, mutedClass } from '@/utils/classes';

export function TeamsPage() {
  const [params, setParams] = useSearchParams();
  const teams = useTeams();
  const name = params.get('team') || (teams.data?.[0]?.name ?? '');
  const team = teams.data?.find((t) => t.name === name);

  return (
    <div className="max-w-3xl space-y-6">
      <h1 className="text-xl font-semibold">Teams</h1>
      {teams.isPending && <p className={mutedClass}>Loading...</p>}
      {teams.isError && <p className={errorClass}>{teams.error.message}</p>}
      <TeamList teams={teams.data ?? []} selected={name} onSelect={(n) => setParams({ team: n })} />
      {team && <TeamEditor key={team.name} team={team} onSaved={(n) => setParams({ team: n })} />}
    </div>
  );
}
