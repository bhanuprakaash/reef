import { useState } from 'react';
import { useSearchParams } from 'react-router';
import { useProviders } from '@/lib/api/get-providers';
import { useSkills } from '@/lib/api/get-skills';
import { useBots } from '@/lib/api/get-bots';
import { useTeams } from '@/lib/api/get-teams';
import { useCurrentProject } from '@/lib/current-project';
import { useProjects } from '@/modules/projects/api/get-projects';
import { NewBotDialog } from '@/modules/teams/components/new-bot-dialog';
import { RoleRow } from '@/modules/teams/components/role-row';
import { BotPanel } from '@/modules/teams/components/bot-panel';
import { Button } from '@/components/button';

export function BotsPage() {
  const [params, setParams] = useSearchParams();
  const bots = useBots();
  const teams = useTeams();
  const projects = useProjects();
  const [project] = useCurrentProject(projects.data);
  const skills = useSkills(project?.id);
  const providers = useProviders();
  const [creating, setCreating] = useState(false);
  const id = Number(params.get('bot'));
  const current = bots.data?.find((t) => t.id === id);

  return (
    <div className="group mx-auto flex w-full max-w-[820px] flex-col gap-6 py-6 md:has-[[aria-label=Bot]]:pr-[444px]">
      <header className="flex flex-wrap items-center gap-3">
        <div>
          <h1 className="text-[32px] font-normal tracking-[-0.02em]">Bots</h1>
          <p className="mt-1 text-[13px] text-muted">Define a Bot once, add it to any team.</p>
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
          New Bot
        </Button>
      </header>
      {bots.isPending && <p className="text-sm text-muted">Loading...</p>}
      {bots.isError && <p className="text-sm text-danger">{bots.error.message}</p>}
      <section aria-label="All bots" className="space-y-1.5">
        {bots.data?.map((t) => (
          <RoleRow
            key={t.id}
            role={t}
            badges={(teams.data ?? []).flatMap((team) =>
              team.roles.filter((r) => r.botId === t.id).map((r) => `${r.name} · ${team.name}`),
            )}
            selected={t.id === id}
            onSelect={() => setParams({ bot: String(t.id) })}
          />
        ))}
      </section>
      {current && (
        <BotPanel
          key={current.id}
          bot={current}
          skills={skills.data}
          providers={providers.data}
          noProject={!project}
          onClose={() => setParams({})}
        />
      )}
      <NewBotDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={(created) => setParams({ bot: String(created) })}
      />
    </div>
  );
}
