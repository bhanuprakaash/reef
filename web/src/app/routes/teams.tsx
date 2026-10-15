import { useState } from 'react';
import { useSearchParams } from 'react-router';
import { useTeams } from '@/lib/api/get-teams';
import { NewTeamDialog } from '@/modules/teams/components/new-team-dialog';
import { TeamEditor } from '@/modules/teams/components/team-editor';
import { TeamList } from '@/modules/teams/components/team-list';
import { Button } from '@/components/button';

export function TeamsPage() {
  const [params, setParams] = useSearchParams();
  const teams = useTeams();
  const [creating, setCreating] = useState<{ from?: string } | null>(null);
  const name = params.get('team') || (teams.data?.[0]?.name ?? '');
  const team = teams.data?.find((t) => t.name === name);

  return (
    <div className="mx-auto flex w-full max-w-[1200px] flex-col gap-6 py-6">
      <header className="flex flex-wrap items-center gap-3">
        <div>
          <h1 className="text-[32px] font-normal tracking-[-0.02em]">Teams</h1>
          <p className="mt-1 text-[13px] text-muted">
            Define a team once, start any channel with it.
          </p>
        </div>
        <Button
          className="ml-auto"
          variant="primary"
          disabled={!teams.data?.length}
          onClick={() => setCreating({})}
        >
          <svg
            className="size-3.5"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2.2"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path d="M12 5v14M5 12h14" />
          </svg>
          New team
        </Button>
      </header>
      {teams.isPending && <p className="text-sm text-muted">Loading...</p>}
      {teams.isError && <p className="text-sm text-danger">{teams.error.message}</p>}
      <div className="flex flex-wrap items-start gap-6">
        <TeamList
          teams={teams.data ?? []}
          selected={name}
          onSelect={(n) => setParams({ team: n })}
        />
        {team && (
          <TeamEditor
            key={team.name}
            team={team}
            onSaved={(n) => setParams({ team: n })}
            onDuplicate={() => setCreating({ from: team.name })}
          />
        )}
      </div>
      <NewTeamDialog
        open={creating !== null}
        from={creating?.from}
        teams={teams.data ?? []}
        onClose={() => setCreating(null)}
        onCreated={(n) => {
          setCreating(null);
          setParams({ team: n });
        }}
      />
    </div>
  );
}
