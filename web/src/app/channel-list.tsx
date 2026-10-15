import { useQuery } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { Link, useLocation } from 'react-router';
import { GroupAvatar } from '@/components/creature';
import { channelsQuery } from '@/lib/api/get-channels';
import { useTeams } from '@/lib/api/get-teams';
import { useFeatures } from '@/modules/features/api/get-features';
import { AddFeatureForm } from '@/modules/features/components/add-feature-form';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import type { ChannelPreview, Project } from '@/types/api';
import { rowTime } from '@/utils/row-time';

export function Row(props: {
  to: string;
  active: boolean;
  avatar: ReactNode;
  title: string;
  aside?: ReactNode;
  preview: string;
}) {
  return (
    <Link
      to={props.to}
      className={`flex items-center gap-3 rounded-xl px-2.5 py-[9px] ${props.active ? 'bg-active' : 'hover:bg-hover/50'}`}
    >
      {props.avatar}
      <span className="min-w-0 flex-1">
        <span className="flex items-baseline gap-2">
          <span className="truncate text-[14.5px]">{props.title}</span>
          <span className="ml-auto shrink-0 text-xs text-muted">{props.aside}</span>
        </span>
        <span className="block truncate text-[13px] text-muted">{props.preview}</span>
      </span>
    </Link>
  );
}

const oneLine = (m: NonNullable<ChannelPreview['lastMessage']>) =>
  `${m.author === 'user' ? 'you' : m.author}: ${m.body.replace(/\s+/g, ' ')}`;

export function ChannelList({ project, query }: { project: Project; query: string }) {
  const { pathname } = useLocation();
  const features = useFeatures();
  const channels = useQuery({ ...channelsQuery, refetchInterval: 5000 });
  const teams = useTeams();
  const inbox = useInbox();

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
  const quick = projectChannels.filter((c) => c.featureId === null && matches(c.name));

  function channelRow(c: ChannelPreview, named: string) {
    const roles = teams.data?.find((t) => t.name === c.team)?.roles.map((r) => r.name);
    return (
      <Row
        to={`/channels/${c.id}`}
        active={pathname === `/channels/${c.id}`}
        avatar={
          <span className="relative flex-none">
            <GroupAvatar roles={roles ?? [c.role]} size={30} />
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
        aside={c.lastMessage && rowTime(c.lastMessage.createdAt)}
        preview={c.lastMessage ? oneLine(c.lastMessage) : 'No messages yet'}
      />
    );
  }

  return (
    <section aria-label={project.name} className="flex flex-col gap-0.5">
      <ul className="flex flex-col gap-0.5">
        {groups.map(({ feature: f, channels: cs }) => (
          <li key={f.id}>
            <div className="flex items-baseline gap-2 px-2.5 pt-3.5 pb-1">
              <Link
                to={`/features/${f.id}`}
                className="truncate text-xs font-medium text-muted hover:text-fg"
              >
                {f.name}
              </Link>
              <span className="text-[11px] text-faint">{f.stage}</span>
            </div>
            <ul className="flex flex-col gap-0.5">
              {cs.map((c) => (
                <li key={c.id}>{channelRow(c, `# ${c.name}`)}</li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
      {quick.length > 0 && (
        <>
          <h4 className="px-2.5 pt-3.5 pb-1 text-xs font-medium text-muted">Quick sessions</h4>
          <ul className="flex flex-col gap-0.5">
            {quick.map((c) => (
              <li key={c.id}>{channelRow(c, c.name)}</li>
            ))}
          </ul>
        </>
      )}
      <div className="px-2.5 pt-3">
        <AddFeatureForm projectId={project.id} />
      </div>
    </section>
  );
}
