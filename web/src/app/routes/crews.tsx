import { useState } from 'react';
import { Navigate, useParams, useSearchParams } from 'react-router';
import { useTeams } from '@/lib/api/get-teams';
import { BotsTabs } from '@/modules/teams/components/bots-tabs';
import { NewTeamDialog } from '@/modules/teams/components/new-team-dialog';
import { TeamEditor } from '@/modules/teams/components/team-editor';
import { TeamList } from '@/modules/teams/components/team-list';
import { BackButton } from '@/components/back-button';
import { Button } from '@/components/button';

export function CrewsTab() {
  const [params, setParams] = useSearchParams();
  const teams = useTeams();
  const [creating, setCreating] = useState<{ from?: string } | null>(null);
  const picked = params.get('team');
  const name = picked || (teams.data?.[0]?.name ?? '');
  const team = teams.data?.find((t) => t.name === name);
  const select = (n: string) => setParams({ tab: 'crews', team: n });

  return (
    <div className="mx-auto flex w-full max-w-[820px] flex-col gap-6 py-6">
      <header className="flex flex-wrap items-center gap-3">
        <div>
          <h1 className="text-[32px] font-normal tracking-[-0.02em]">Bots</h1>
          <p className="mt-1 text-[13px] text-muted">
            A Crew is a reusable list of Bots. Start a Space with one or more.
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
          New crew
        </Button>
      </header>
      <BotsTabs />
      {teams.isPending && <p className="text-sm text-muted">Loading...</p>}
      {teams.isError && <p className="text-sm text-danger">{teams.error.message}</p>}
      <div className="flex items-start gap-8">
        <div className={picked ? 'max-md:hidden' : 'max-md:w-full'}>
          <TeamList teams={teams.data ?? []} selected={name} onSelect={select} />
        </div>
        {team && (
          <div className={`flex min-w-0 flex-1 flex-col gap-3 ${picked ? '' : 'max-md:hidden'}`}>
            <span className="md:hidden">
              <BackButton fallback="/bots?tab=crews" />
            </span>
            <TeamEditor
              key={team.name}
              team={team}
              onDeleted={() => select('solo')}
              onDuplicate={() => setCreating({ from: team.name })}
            />
          </div>
        )}
      </div>
      <NewTeamDialog
        open={creating !== null}
        from={creating?.from}
        teams={teams.data ?? []}
        onClose={() => setCreating(null)}
        onCreated={(n) => {
          setCreating(null);
          select(n);
        }}
      />
    </div>
  );
}

export function TeamsRedirect() {
  const { name } = useParams();
  const [params] = useSearchParams();
  const team = name ?? params.get('team');
  return (
    <Navigate replace to={`/bots?tab=crews${team ? `&team=${encodeURIComponent(team)}` : ''}`} />
  );
}
