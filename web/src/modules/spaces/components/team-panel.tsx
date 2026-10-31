import { useState } from 'react';
import { Link } from 'react-router';
import { useRefreshBot } from '@/modules/spaces/api/refresh-bot';
import { useSetCoordinator } from '@/modules/spaces/api/set-coordinator';
import { useInspector } from '@/modules/spaces/api/get-inspector';
import { formatTokens } from '@/modules/spaces/utils/format-tokens';
import { SpaceProjects } from '@/modules/spaces/components/space-projects';
import type { Project, Role, RoleSession } from '@/types/api';
import { Button } from '@/components/button';
import { Chip } from '@/components/chip';
import { Markdown } from '@/components/markdown';
import { TextField } from '@/components/text-field';
import { Creature, type CreatureState } from '@/components/creature';

type Props = {
  spaceId: number;
  workItemId: number | null;
  roles: Role[];
  projects: Project[];
  working: number | null;
  brief?: string;
  onClose: () => void;
};

const backendLabel: Record<string, string> = { claude: 'Claude Code', codex: 'Codex' };

const stateLabel: Record<CreatureState, string> = {
  working: 'working',
  'needs-you': 'waiting on you',
  asleep: 'asleep',
  idle: 'idle',
  failed: 'failed',
};

function SessionSummary({ session }: { session: RoleSession }) {
  const incompatible = session.skills.filter((s) => !s.compatible);
  const facts = [
    session.model || 'default model',
    session.permissionMode || 'default permissions',
    session.write ? 'can write' : 'read-only',
    session.tools.length > 0 ? `tools: ${session.tools.join(', ')}` : '',
    session.skills.length > 0 ? `skills: ${session.skills.map((s) => s.ref).join(', ')}` : '',
    session.connections.length > 0 ? `connections: ${session.connections.join(', ')}` : '',
  ].filter(Boolean);
  return (
    <div className="basis-full pl-[42px] text-[12.5px] text-muted">
      <p>Session runs with {facts.join(' · ')}</p>
      {incompatible.map((s) => (
        <p key={s.ref}>
          {s.ref} is not available here: {s.reason}
        </p>
      ))}
      {session.refreshPending ? (
        <p>Refresh applies before this Bot's next turn.</p>
      ) : (
        session.stale && <p>Bot settings changed. Refresh to adopt edits.</p>
      )}
    </div>
  );
}

export function TeamPanel({
  spaceId,
  workItemId,
  roles,
  projects,
  working,
  brief,
  onClose,
}: Props) {
  const inspector = useInspector(spaceId, true);
  const setCoordinator = useSetCoordinator(spaceId);
  const refresh = useRefreshBot(spaceId);
  const [models, setModels] = useState<Record<number, string>>({});

  function stateOf(role: Role): CreatureState {
    if (role.botId === working) return 'working';
    return role.lead ? 'idle' : 'asleep';
  }

  return (
    <aside aria-label="Bots" className="space-y-6">
      <div>
        <div className="mb-2 flex items-center">
          <h2 className="text-[13px] text-muted">Bots</h2>
          <button
            type="button"
            aria-label="Close Bots"
            onClick={onClose}
            className="ml-auto flex size-8 items-center justify-center rounded-field text-soft hover:bg-card max-md:hidden"
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M6 6l12 12M18 6L6 18" />
            </svg>
          </button>
        </div>
        <ul className="space-y-0.5">
          {roles.map((role) => {
            const state = stateOf(role);
            const shown = role.session ?? role;
            const usage = inspector.data?.roles.find((r) => r.botId === role.botId)?.usage;
            return (
              <li
                key={role.botId}
                className={`flex flex-wrap items-center gap-3 rounded-field px-1.5 py-2 ${state === 'working' ? 'bg-card' : ''}`}
              >
                <Creature look={role} state={state} size={30} animate />
                <span className="min-w-0 flex-1">
                  <span
                    className={`flex items-center gap-2 text-[14.5px] ${state === 'asleep' ? 'text-muted' : ''}`}
                  >
                    <Link to={`/bots?bot=${role.botId}`} className="truncate capitalize">
                      {role.name}
                    </Link>
                    {role.lead && <Chip className="h-5 px-2 text-[11px]">coordinator</Chip>}
                  </span>
                  <span className="block text-[12.5px] text-muted">
                    {[backendLabel[shown.backend] ?? shown.backend, stateLabel[state]]
                      .filter(Boolean)
                      .join(' · ')}
                  </span>
                </span>
                {!role.lead && (
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={setCoordinator.isPending}
                    onClick={() => setCoordinator.mutate(role.botId)}
                  >
                    Make coordinator
                  </Button>
                )}
                {usage && (
                  <span className="text-[12.5px] tabular-nums text-muted">
                    {formatTokens(usage.inputTokens + usage.outputTokens)} tokens
                  </span>
                )}
                {role.session && <SessionSummary session={role.session} />}
                <div className="flex basis-full gap-2 pl-[42px]">
                  <TextField
                    aria-label={`Model for ${role.name}`}
                    placeholder={shown.model || 'model'}
                    value={models[role.botId] ?? ''}
                    onChange={(e) => setModels({ ...models, [role.botId]: e.target.value })}
                    className="w-36"
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={refresh.isPending}
                    onClick={() => refresh.mutate({ botId: role.botId, model: models[role.botId] })}
                  >
                    Refresh
                  </Button>
                </div>
              </li>
            );
          })}
        </ul>
        {refresh.isError && <p className="mt-1.5 text-sm text-danger">{refresh.error.message}</p>}
        {refresh.isSuccess && (
          <p className="mt-1.5 text-sm text-muted">
            {refresh.data.pending
              ? "Refresh applies before this Bot's next turn."
              : 'Refresh requested: the next message starts a new session.'}
          </p>
        )}
        {setCoordinator.isError && (
          <p className="mt-1.5 text-sm text-danger">{setCoordinator.error.message}</p>
        )}
      </div>
      <SpaceProjects spaceId={spaceId} projects={projects} readOnly={workItemId !== null} />
      {workItemId !== null && (
        <div>
          <h2 className="mb-1.5 text-[13px] text-muted">Brief</h2>
          <div className="text-[13.5px] text-soft">
            <Markdown>{brief || 'No brief yet.'}</Markdown>
          </div>
        </div>
      )}
    </aside>
  );
}
