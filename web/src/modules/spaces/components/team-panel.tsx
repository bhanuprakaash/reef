import { useState, type ReactNode } from 'react';
import { Link } from 'react-router';
import { useRefreshBot } from '@/modules/spaces/api/refresh-bot';
import { useRemoveMember } from '@/modules/spaces/api/remove-member';
import { useSetCoordinator } from '@/modules/spaces/api/set-coordinator';
import { useInspector } from '@/modules/spaces/api/get-inspector';
import { formatTokens } from '@/modules/spaces/utils/format-tokens';
import { SpaceFixes } from '@/modules/spaces/components/space-fixes';
import { SpaceProjects } from '@/modules/spaces/components/space-projects';
import type { Project, Role } from '@/types/api';
import { Button, buttonClass } from '@/components/button';
import { Chip } from '@/components/chip';
import { Markdown } from '@/components/markdown';
import { Select } from '@/components/select';
import { StatusDot, type Status } from '@/components/status-dot';
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

const stateDot: Record<CreatureState, Status> = {
  working: 'working',
  'needs-you': 'needs-you',
  asleep: 'asleep',
  idle: 'done',
  failed: 'failed',
};

const icon = (path: string) => (
  <svg
    width="16"
    height="16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
  >
    <path d={path} />
  </svg>
);

const iconButton =
  'flex size-8 items-center justify-center rounded-field text-soft hover:bg-card max-md:hidden';

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="mt-5">
      <h3 className="mb-1.5 text-[11px] uppercase tracking-[0.08em] text-muted">{title}</h3>
      <div className="divide-y divide-hairline rounded-[14px] bg-card px-3.5">{children}</div>
    </section>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-h-11 items-center justify-between gap-3 py-1.5 text-[13.5px]">
      <span className="text-soft">{label}</span>
      <span className="flex min-w-0 flex-wrap justify-end gap-1.5 text-right text-soft">
        {children}
      </span>
    </div>
  );
}

function Chips({ label, values }: { label: string; values: string[] }) {
  const [all, setAll] = useState(false);
  const shown = all ? values : values.slice(0, 3);
  return (
    <div className="py-2.5">
      <div className="mb-1.5 flex items-center justify-between text-[13.5px]">
        <span className="text-soft">{label}</span>
        <span className="text-xs text-muted">{values.length}</span>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {shown.map((v) => (
          <Chip key={v} className="h-5 px-2 text-[11px]">
            {v}
          </Chip>
        ))}
        {values.length > shown.length && (
          <button
            type="button"
            aria-expanded={all}
            onClick={() => setAll(true)}
            className="h-5 rounded-pill bg-bubble px-2 text-[11px] text-soft hover:bg-hover"
          >
            +{values.length - shown.length}
          </button>
        )}
        {values.length === 0 && <span className="text-xs text-muted">None</span>}
      </div>
    </div>
  );
}

function Access({ role }: { role: Role }) {
  const session = role.session;
  const incompatible = session?.skills.filter((s) => !s.compatible) ?? [];
  return (
    <Group title="Access">
      <Chips label="Tools" values={session?.tools ?? role.tools} />
      <Chips label="Skills" values={session ? session.skills.map((s) => s.ref) : role.skills} />
      {session && <Chips label="Connections" values={session.connections} />}
      {incompatible.map((s) => (
        <p key={s.ref} className="py-2.5 text-xs text-muted">
          {s.ref} is not available here: {s.reason}
        </p>
      ))}
    </Group>
  );
}

export function BotProfile({
  spaceId,
  role,
  roles,
  state,
  tokens,
  onBack,
  onClose,
}: {
  spaceId: number;
  role: Role;
  roles: Role[];
  state: CreatureState;
  tokens: number | undefined;
  onBack: () => void;
  onClose: () => void;
}) {
  const setCoordinator = useSetCoordinator(spaceId);
  const refresh = useRefreshBot(spaceId);
  const remove = useRemoveMember(spaceId);
  const [model, setModel] = useState<string>();
  const [successor, setSuccessor] = useState<number>();
  const shown = role.session ?? role;
  const others = roles.filter((r) => r.botId !== role.botId);
  const next = successor ?? others[0]?.botId;
  const summary = role.description.split('\n')[0];

  return (
    <>
      <div className="mb-3 flex items-center">
        <button
          type="button"
          aria-label="Back to Bots"
          onClick={onBack}
          className="-ml-1 flex h-8 items-center gap-1 rounded-field pr-2 pl-1 text-[13.5px] text-soft hover:bg-card"
        >
          {icon('M15 5l-7 7 7 7')}
          Bots
        </button>
        <button
          type="button"
          aria-label="Close Bots"
          onClick={onClose}
          className={`ml-auto ${iconButton}`}
        >
          {icon('M6 6l12 12M18 6L6 18')}
        </button>
      </div>
      <div className="flex items-center gap-3.5">
        <Creature look={role} state={state} size={48} animate />
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-xl capitalize">{role.name}</h2>
            {role.lead && <Chip className="h-5 px-2 text-[11px]">coordinator</Chip>}
          </div>
          <div className="mt-0.5 flex items-center gap-1.5 text-[13px] text-muted">
            <StatusDot status={stateDot[state]} />
            <span className="truncate">
              {stateLabel[state]}
              {summary && ` · ${summary}`}
            </span>
          </div>
        </div>
      </div>
      <Group title="Session">
        <Row label="Model">
          {shown.backend === 'claude' ? (
            <Select
              variant="pill"
              aria-label={`Model for ${role.name}`}
              value={model ?? shown.model ?? ''}
              options={claudeModelOptions(shown.model ?? '').filter((o) => o.value || !shown.model)}
              onChange={setModel}
            />
          ) : (
            <TextField
              aria-label={`Model for ${role.name}`}
              placeholder={shown.model || 'model'}
              value={model ?? ''}
              onChange={(e) => setModel(e.target.value)}
              className="w-36"
            />
          )}
        </Row>
        <Row label="Permissions">
          {shown.permissionMode || 'default permissions'} ·{' '}
          {shown.write ? 'can write' : 'read-only'}
        </Row>
        <Row label="Backend">{backendLabel[shown.backend] ?? shown.backend}</Row>
        {tokens !== undefined && (
          <Row label="Usage">{formatTokens(tokens)} tokens in this Space</Row>
        )}
      </Group>
      <Access role={role} />
      <p className="mt-4 text-xs text-muted">
        {role.session?.refreshPending
          ? "Apply here runs before this Bot's next turn."
          : role.session?.stale
            ? 'Bot settings changed. Apply here to adopt edits.'
            : "Changes apply to new chats. Apply here restarts this Bot's session in this chat with the new settings and keeps its history."}
      </p>
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
      <div className="sticky -bottom-4 z-10 -mx-4 -mb-4 mt-4 space-y-2 bg-field px-4 py-3 max-md:-bottom-8 max-md:-mb-8 max-md:pb-8 md:rounded-b-card">
        {role.lead && !role.leaving && others.length > 0 && (
          <div className="flex items-center justify-between gap-3 text-[13px] text-muted">
            Coordinator after {role.name} leaves
            <Select
              aria-label={`Coordinator after ${role.name} leaves`}
              value={String(next)}
              onChange={(v) => setSuccessor(Number(v))}
              options={others.map((r) => ({ value: String(r.botId), label: r.name }))}
              className="w-32"
            />
          </div>
        )}
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="primary"
            size="sm"
            disabled={refresh.isPending}
            onClick={() => refresh.mutate({ botId: role.botId, model })}
          >
            Apply here
          </Button>
          {!role.lead && (
            <Button
              size="sm"
              disabled={setCoordinator.isPending}
              onClick={() => setCoordinator.mutate(role.botId)}
            >
              Make coordinator
            </Button>
          )}
          <Link to={`/bots?bot=${role.botId}`} className={buttonClass('secondary', 'sm')}>
            Edit Bot
          </Link>
          {others.length > 0 &&
            (role.leaving ? (
              <span className="text-[12.5px] text-muted">Leaves after its current turn.</span>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                className="text-danger!"
                disabled={remove.isPending}
                onClick={() =>
                  remove.mutate({ botId: role.botId, coordinator: role.lead ? next : undefined })
                }
              >
                Remove
              </Button>
            ))}
        </div>
      </div>
    </>
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
  const [selected, setSelected] = useState<number>();
  const role = roles.find((r) => r.botId === selected);

  function stateOf(r: Role): CreatureState {
    if (r.botId === working) return 'working';
    return r.lead ? 'idle' : 'asleep';
  }

  function usageOf(botId: number) {
    const usage = inspector.data?.roles.find((r) => r.botId === botId)?.usage;
    return usage && usage.inputTokens + usage.outputTokens;
  }

  return (
    <aside aria-label="Bots" className={role ? undefined : 'space-y-6'}>
      {role ? (
        <BotProfile
          key={role.botId}
          spaceId={spaceId}
          role={role}
          roles={roles}
          state={stateOf(role)}
          tokens={usageOf(role.botId)}
          onBack={() => setSelected(undefined)}
          onClose={onClose}
        />
      ) : (
        <>
          <div>
            <div className="mb-2 flex items-center">
              <h2 className="text-[13px] text-muted">
                Bots <span>{roles.length}</span>
              </h2>
              <button
                type="button"
                aria-label="Close Bots"
                onClick={onClose}
                className={`ml-auto ${iconButton}`}
              >
                {icon('M6 6l12 12M18 6L6 18')}
              </button>
            </div>
            <ul className="space-y-0.5">
              {roles.map((r) => {
                const state = stateOf(r);
                const shown = r.session ?? r;
                return (
                  <li key={r.botId}>
                    <button
                      type="button"
                      aria-label={`Open ${r.name}`}
                      onClick={() => setSelected(r.botId)}
                      className={`flex w-full items-center gap-3 rounded-field px-1.5 py-2 text-left hover:bg-card ${state === 'working' ? 'bg-card' : ''}`}
                    >
                      <Creature look={r} state={state} size={30} animate />
                      <span className="min-w-0 flex-1">
                        <span
                          className={`flex items-center gap-2 text-[14.5px] ${state === 'asleep' ? 'text-muted' : ''}`}
                        >
                          <span className="truncate capitalize">{r.name}</span>
                          {r.lead && <Chip className="h-5 px-2 text-[11px]">coordinator</Chip>}
                        </span>
                        <span className="block text-[12.5px] text-muted">
                          {backendLabel[shown.backend] ?? shown.backend} · {stateLabel[state]}
                        </span>
                      </span>
                      {icon('M9 5l7 7-7 7')}
                    </button>
                  </li>
                );
              })}
            </ul>
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
        </>
      )}
    </aside>
  );
}
