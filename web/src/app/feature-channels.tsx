import { useQuery } from '@tanstack/react-query';
import { type ReactNode, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';
import { FeatureStageChip } from '@/app/feature-stage';
import { GroupAvatar } from '@/components/creature';
import { Dialog } from '@/components/dialog';
import { Row } from '@/components/row';
import { StatusDot } from '@/components/status-dot';
import { channelsQuery } from '@/lib/api/get-channels';
import { useTeams } from '@/lib/api/get-teams';
import { useRunningTurns } from '@/modules/channels/api/get-turns';
import { NewChannelForm } from '@/modules/channels/components/new-channel-form';
import { useFeatures } from '@/modules/features/api/get-features';
import { AddFeatureForm } from '@/modules/features/components/add-feature-form';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import { useTasks } from '@/modules/tasks/api/get-tasks';
import type { ChannelPreview, Feature, Project } from '@/types/api';
import { roleFaces } from '@/utils/role-faces';
import { rowTime } from '@/utils/row-time';

const COLLAPSED_KEY = 'reef.sidebar.collapsed';

function storedCollapsed(): number[] {
  try {
    return JSON.parse(localStorage.getItem(COLLAPSED_KEY) ?? '[]') as number[];
  } catch {
    return [];
  }
}

const oneLine = (m: NonNullable<ChannelPreview['lastMessage']>) =>
  `${m.author === 'user' ? 'you' : m.author}: ${m.body.replace(/\s+/g, ' ')}`;

function Fold({
  label,
  count,
  open,
  onToggle,
  className = '',
  children,
}: {
  label: string;
  count: number;
  open: boolean;
  onToggle: () => void;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={className}>
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="flex h-7 w-full items-center gap-1.5 rounded-md pl-2.5 text-xs text-muted hover:text-fg max-md:h-11"
      >
        <svg
          width="10"
          height="10"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.4"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className={`transition-transform ${open ? 'rotate-90' : ''}`}
        >
          <path d="M9 6l6 6-6 6" />
        </svg>
        {label} ({count})
      </button>
      {open && children}
    </div>
  );
}

function AddButton({
  label,
  onClick,
  reveal,
}: {
  label: string;
  onClick: () => void;
  reveal?: boolean;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      className={`flex h-6 flex-none items-center justify-center overflow-hidden rounded-md text-muted hover:bg-hover hover:text-fg max-md:size-11 ${reveal ? 'md:w-0 md:opacity-0 md:group-hover/h:w-6 md:group-hover/h:opacity-100 md:focus-visible:w-6 md:focus-visible:opacity-100' : 'ml-auto md:w-6'}`}
    >
      <svg
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        aria-hidden="true"
      >
        <path d="M12 4v16M4 12h16" />
      </svg>
    </button>
  );
}

export function FeatureChannels({ project, query }: { project: Project; query: string }) {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const [addingFeature, setAddingFeature] = useState(false);
  const [channelFeature, setChannelFeature] = useState<Feature | null>(null);
  const [collapsed, setCollapsed] = useState(storedCollapsed);
  const [wrappedOpen, setWrappedOpen] = useState<number[]>([]);
  const [closedOpen, setClosedOpen] = useState(false);

  function toggle(id: number) {
    const next = collapsed.includes(id) ? collapsed.filter((x) => x !== id) : [...collapsed, id];
    setCollapsed(next);
    try {
      localStorage.setItem(COLLAPSED_KEY, JSON.stringify(next));
    } catch {
      // blocked storage only loses the remembered state
    }
  }
  function toggleWrapped(id: number) {
    setWrappedOpen(
      wrappedOpen.includes(id) ? wrappedOpen.filter((x) => x !== id) : [...wrappedOpen, id],
    );
  }
  const features = useFeatures();
  const channels = useQuery({ ...channelsQuery, refetchInterval: 5000 });
  const teams = useTeams();
  const inbox = useInbox();
  const tasks = useTasks();
  const running = useRunningTurns();

  const waiting = inbox.data ?? [];
  const q = query.trim().toLowerCase();
  const matches = (name: string) => name.toLowerCase().includes(q);

  const projectChannels = channels.data?.filter((c) => c.projectId === project.id) ?? [];
  const groups = (features.data ?? [])
    .filter((f) => f.projectId === project.id)
    .map((f) => ({
      feature: f,
      channels: projectChannels.filter(
        (c) => c.featureId === f.id && (matches(f.name) || matches(c.name)),
      ),
    }))
    .filter((g) => matches(g.feature.name) || g.channels.length > 0);
  const openGroups = groups.filter((g) => g.feature.closedAt === null);
  const closedGroups = groups.filter((g) => g.feature.closedAt !== null);
  const quick = projectChannels.filter((c) => c.featureId === null && matches(c.name));

  const working = new Map((running.data ?? []).map((t) => [t.channelId, t.role]));

  function channelRow(c: ChannelPreview, named: string) {
    const faces = roleFaces(teams.data, c.team);
    const roles = Object.keys(faces);
    const role = working.get(c.id);
    return (
      <Row
        to={`/channels/${c.id}`}
        active={pathname === `/channels/${c.id}`}
        avatar={
          <span className="relative flex-none">
            <GroupAvatar roles={roles.length > 0 ? roles : [c.role]} faces={faces} size={30} />
            {waiting.some((i) => i.channelId === c.id) && (
              <span
                role="img"
                aria-label="needs you"
                className="absolute -top-0.5 -right-0.5 size-2.5 rounded-full border-2 border-sidebar bg-needs-you"
              />
            )}
          </span>
        }
        title={named}
        live={role ? <StatusDot status="working" label={`${role} is working`} /> : undefined}
        aside={c.lastMessage && rowTime(c.lastMessage.createdAt)}
        preview={c.lastMessage ? oneLine(c.lastMessage) : 'No messages yet'}
      />
    );
  }

  function featureGroup({ feature: f, channels: cs }: (typeof groups)[number]) {
    const open = q !== '' || !collapsed.includes(f.id);
    const wrapped = f.closedAt === null ? cs.filter((c) => c.closedAt !== null) : [];
    const live = cs.filter((c) => !wrapped.includes(c));
    return (
      <li key={f.id}>
        <div
          className={`group/h mt-1.5 mb-0.5 flex h-9 items-center gap-1 rounded-xl pr-1.5 pl-1 ${pathname === `/features/${f.id}` ? 'bg-active' : 'hover:bg-hover/50'}`}
        >
          <button
            type="button"
            aria-label={`${open ? 'Collapse' : 'Expand'} ${f.name}`}
            aria-expanded={open}
            onClick={() => toggle(f.id)}
            className="flex size-6 flex-none items-center justify-center rounded-md text-faint hover:bg-hover hover:text-fg max-md:size-11"
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
              className={`transition-transform ${open ? 'rotate-90' : ''}`}
            >
              <path d="M9 6l6 6-6 6" />
            </svg>
          </button>
          <Link
            to={`/features/${f.id}`}
            title={f.kind === 'fixes' ? f.name : `${f.name} · ${f.stage}`}
            aria-label={f.name}
            className="flex min-w-0 flex-1 items-center gap-2 self-stretch text-[13px] text-soft hover:text-fg"
          >
            <span className="truncate">{f.name}</span>
            {!open && cs.length > 0 && (
              <span className="flex-none text-[11px] text-faint">{cs.length}</span>
            )}
            <span className="ml-auto">
              <FeatureStageChip feature={f} tasks={tasks.data ?? []} />
            </span>
          </Link>
          <AddButton
            reveal
            label={`New channel in ${f.name}`}
            onClick={() => setChannelFeature(f)}
          />
        </div>
        {open && (
          <>
            <ul className="flex flex-col gap-0.5">
              {live.map((c) => (
                <li key={c.id}>{channelRow(c, `#${c.name}`)}</li>
              ))}
            </ul>
            {wrapped.length > 0 && (
              <Fold
                label="Wrapped"
                count={wrapped.length}
                open={q !== '' || wrappedOpen.includes(f.id)}
                onToggle={() => toggleWrapped(f.id)}
              >
                <ul className="flex flex-col gap-0.5">
                  {wrapped.map((c) => (
                    <li key={c.id}>{channelRow(c, `#${c.name}`)}</li>
                  ))}
                </ul>
              </Fold>
            )}
          </>
        )}
      </li>
    );
  }

  return (
    <section aria-label={project.name} className="flex flex-col gap-0.5">
      <div className="group/h flex items-center gap-2 pt-3.5 pr-1 pb-1 pl-2.5">
        <h4 className="text-xs text-muted">Features</h4>
        <AddButton label="New feature" onClick={() => setAddingFeature(true)} />
      </div>
      {groups.length === 0 && <p className="px-2.5 pb-1 text-[13px] text-faint">No features yet</p>}
      <ul className="flex flex-col gap-0.5">{openGroups.map(featureGroup)}</ul>
      {quick.length > 0 && (
        <>
          <h4 className="px-2.5 pt-3.5 pb-1 text-xs text-muted">Quick sessions</h4>
          <ul className="flex flex-col gap-0.5">
            {quick.map((c) => (
              <li key={c.id}>{channelRow(c, c.name)}</li>
            ))}
          </ul>
        </>
      )}
      {closedGroups.length > 0 && (
        <Fold
          label="Closed"
          count={closedGroups.length}
          open={q !== '' || closedOpen}
          onToggle={() => setClosedOpen(!closedOpen)}
          className="pt-3.5"
        >
          <ul className="flex flex-col gap-0.5">{closedGroups.map(featureGroup)}</ul>
        </Fold>
      )}
      <Dialog open={addingFeature} onClose={() => setAddingFeature(false)} title="New feature">
        {addingFeature && (
          <AddFeatureForm
            projectId={project.id}
            onClose={() => setAddingFeature(false)}
            onCreated={(f) => void navigate(`/features/${f.id}`)}
          />
        )}
      </Dialog>
      <Dialog
        open={channelFeature !== null}
        onClose={() => setChannelFeature(null)}
        title={`New channel in ${channelFeature?.name ?? ''}`}
      >
        {channelFeature && (
          <NewChannelForm
            projectId={project.id}
            featureId={channelFeature.id}
            onClose={() => setChannelFeature(null)}
          />
        )}
      </Dialog>
    </section>
  );
}
