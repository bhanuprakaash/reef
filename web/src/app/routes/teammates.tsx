import { useState } from 'react';
import { useSearchParams } from 'react-router';
import { useProviders } from '@/lib/api/get-providers';
import { useSkills } from '@/lib/api/get-skills';
import { useTeammates } from '@/lib/api/get-teammates';
import { useTeams } from '@/lib/api/get-teams';
import { useCurrentProject } from '@/lib/current-project';
import { useProjects } from '@/modules/projects/api/get-projects';
import { NewTeammateDialog } from '@/modules/teams/components/new-teammate-dialog';
import { RoleRow } from '@/modules/teams/components/role-row';
import { TeammatePanel } from '@/modules/teams/components/teammate-panel';
import { Button } from '@/components/button';

export function TeammatesPage() {
  const [params, setParams] = useSearchParams();
  const teammates = useTeammates();
  const teams = useTeams();
  const projects = useProjects();
  const [project] = useCurrentProject(projects.data);
  const skills = useSkills(project?.id);
  const providers = useProviders();
  const [creating, setCreating] = useState(false);
  const id = Number(params.get('teammate'));
  const current = teammates.data?.find((t) => t.id === id);

  return (
    <div className="group mx-auto flex w-full max-w-[820px] flex-col gap-6 py-6 md:has-[[aria-label=Teammate]]:pr-[444px]">
      <header className="flex flex-wrap items-center gap-3">
        <div>
          <h1 className="text-[32px] font-normal tracking-[-0.02em]">Teammates</h1>
          <p className="mt-1 text-[13px] text-muted">Define a teammate once, add it to any team.</p>
        </div>
        <Button className="ml-auto" variant="primary" onClick={() => setCreating(true)}>
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
          New teammate
        </Button>
      </header>
      {teammates.isPending && <p className="text-sm text-muted">Loading...</p>}
      {teammates.isError && <p className="text-sm text-danger">{teammates.error.message}</p>}
      <section aria-label="All teammates" className="space-y-1.5">
        {teammates.data?.map((t) => (
          <RoleRow
            key={t.id}
            role={t}
            badges={(teams.data ?? []).flatMap((team) =>
              team.roles
                .filter((r) => r.teammateId === t.id)
                .map((r) => `${r.name} · ${team.name}`),
            )}
            selected={t.id === id}
            onSelect={() => setParams({ teammate: String(t.id) })}
          />
        ))}
      </section>
      {current && (
        <TeammatePanel
          key={current.id}
          teammate={current}
          skills={skills.data}
          providers={providers.data}
          noProject={!project}
          onClose={() => setParams({})}
        />
      )}
      <NewTeammateDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={(created) => setParams({ teammate: String(created) })}
      />
    </div>
  );
}
