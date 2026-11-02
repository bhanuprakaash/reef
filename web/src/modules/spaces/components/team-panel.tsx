import { useState, type ReactNode } from 'react';
import { Link } from 'react-router';
import { useRefreshBot } from '@/modules/spaces/api/refresh-bot';
import { useRemoveMember } from '@/modules/spaces/api/remove-member';
import { useSetCoordinator } from '@/modules/spaces/api/set-coordinator';
import { useInspector } from '@/modules/spaces/api/get-inspector';
import { formatTokens } from '@/modules/spaces/utils/format-tokens';
import { SpaceFixes } from '@/modules/spaces/components/space-fixes';
import { SpaceProjects } from '@/modules/spaces/components/space-projects';
import type { Project, Role, RoleSession } from '@/types/api';
import { Button } from '@/components/button';
import { Chip } from '@/components/chip';
import { Markdown } from '@/components/markdown';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';
import { claudeModelOptions } from '@/utils/models';
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

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt>{label}</dt>
      <dd className="min-w-0 break-words text-soft">{children}</dd>
    </>
  );
}

function Collapsed({ values, noun }: { values: string[]; noun: string }) {
  const [open, setOpen] = useState(false);
  if (values.length <= 3) return values.join(', ');
  return (
    <>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="h-5 rounded-pill bg-bubble px-2.5 text-[11px] text-soft hover:bg-hover"
      >
        {values.length} {noun}
      </button>
      {open && <span className="mt-1 block">{values.join(', ')}</span>}
    </>
  );
}

function SessionSummary({ session }: { session: RoleSession }) {
  const incompatible = session.skills.filter((s) => !s.compatible);
  return (
    <div className="basis-full pl-[42px] text-[12.5px] text-muted">
      <dl className="grid grid-cols-[84px_1fr] gap-y-1">
        <Detail label="Model">{session.model || 'default model'}</Detail>
        <Detail label="Permissions">
          {session.permissionMode || 'default permissions'} ·{' '}
          {session.write ? 'can write' : 'read-only'}
        </Detail>
        {session.tools.length > 0 && (
          <Detail label="Tools">
            <Collapsed values={session.tools} noun="tools" />
          </Detail>
        )}
        {session.skills.length > 0 && (
          <Detail label="Skills">
            <Collapsed values={session.skills.map((s) => s.ref)} noun="skills" />
          </Detail>
        )}
        {session.connections.length > 0 && (
          <Detail label="Connections">
            <Collapsed values={session.connections} noun="connections" />
          </Detail>
        )}
      </dl>
      {incompatible.map((s) => (
        <p key={s.ref}>
          {s.ref} is not available here: {s.reason}
        </p>
      ))}
      {session.refreshPending ? (
        <p>Apply here runs before this Bot's next turn.</p>
      ) : (
        session.stale && <p>Bot settings changed. Apply here to adopt edits.</p>
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
  const remove = useRemoveMember(spaceId);
  const [models, setModels] = useState<Record<number, string>>({});
  const [successors, setSuccessors] = useState<Record<number, number>>({});

  function successorOf(role: Role): number {
    return successors[role.botId] ?? roles.find((r) => r.botId !== role.botId)!.botId;
  }

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
        <p className="mb-2 text-xs text-muted">
          Changes apply to new chats. Apply here restarts this Bot's session in this chat with the
          new settings and keeps its history.
        </p>
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
                    {[
                      backendLabel[shown.backend] ?? shown.backend,
                      stateLabel[state],
                      usage && `${formatTokens(usage.inputTokens + usage.outputTokens)} tokens`,
                    ]
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
                {role.session && <SessionSummary session={role.session} />}
                <div className="flex basis-full flex-wrap items-center gap-2 pl-[42px]">
                  {shown.backend === 'claude' ? (
                    <Select
                      variant="pill"
                      aria-label={`Model for ${role.name}`}
                      value={models[role.botId] ?? shown.model ?? ''}
                      options={claudeModelOptions(shown.model ?? '').filter(
                        (o) => o.value || !shown.model,
                      )}
                      onChange={(model) => setModels({ ...models, [role.botId]: model })}
                    />
                  ) : (
                    <TextField
                      aria-label={`Model for ${role.name}`}
                      placeholder={shown.model || 'model'}
                      value={models[role.botId] ?? ''}
                      onChange={(e) => setModels({ ...models, [role.botId]: e.target.value })}
                      className="w-36"
                    />
                  )}
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={refresh.isPending}
                    onClick={() => refresh.mutate({ botId: role.botId, model: models[role.botId] })}
                  >
                    Apply here
                  </Button>
                  {roles.length > 1 &&
                    (role.leaving ? (
                      <span className="text-[12.5px] text-muted">
                        Leaves after its current turn.
                      </span>
                    ) : (
                      <>
                        {role.lead && (
                          <Select
                            aria-label={`Coordinator after ${role.name} leaves`}
                            value={String(successorOf(role))}
                            onChange={(v) =>
                              setSuccessors({ ...successors, [role.botId]: Number(v) })
                            }
                            options={roles
                              .filter((r) => r.botId !== role.botId)
                              .map((r) => ({ value: String(r.botId), label: r.name }))}
                            className="w-32"
                          />
                        )}
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={remove.isPending}
                          onClick={() =>
                            remove.mutate({
                              botId: role.botId,
                              coordinator: role.lead ? successorOf(role) : undefined,
                            })
                          }
                        >
                          Remove
                        </Button>
                      </>
                    ))}
                </div>
              </li>
            );
          })}
        </ul>
        {refresh.isError && <p className="mt-1.5 text-sm text-danger">{refresh.error.message}</p>}
        {refresh.isSuccess && (
          <p className="mt-1.5 text-sm text-muted">
            {refresh.data.pending
              ? "Apply here runs before this Bot's next turn."
              : 'Applied: the next message starts a new session.'}
          </p>
        )}
        {remove.isError && <p className="mt-1.5 text-sm text-danger">{remove.error.message}</p>}
        {setCoordinator.isError && (
          <p className="mt-1.5 text-sm text-danger">{setCoordinator.error.message}</p>
        )}
      </div>
      <SpaceProjects spaceId={spaceId} projects={projects} readOnly={workItemId !== null} />
      {workItemId === null && <SpaceFixes spaceId={spaceId} />}
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
