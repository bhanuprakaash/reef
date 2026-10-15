import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';
import { GroupAvatar } from '@/components/creature';
import { Dialog } from '@/components/dialog';
import { Row } from '@/components/row';
import { channelsQuery } from '@/lib/api/get-channels';
import { useTeams } from '@/lib/api/get-teams';
import { NewChannelForm } from '@/modules/channels/components/new-channel-form';
import { useFeatures } from '@/modules/features/api/get-features';
import { AddFeatureForm } from '@/modules/features/components/add-feature-form';
import { useInbox } from '@/modules/inbox/api/get-inbox';
import type { ChannelPreview, Feature, Project } from '@/types/api';
import { roleFaces } from '@/utils/role-faces';
import { rowTime } from '@/utils/row-time';

const oneLine = (m: NonNullable<ChannelPreview['lastMessage']>) =>
  `${m.author === 'user' ? 'you' : m.author}: ${m.body.replace(/\s+/g, ' ')}`;

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
      className={`ml-auto flex size-6 flex-none items-center justify-center rounded-md text-muted hover:bg-hover hover:text-fg ${reveal ? 'opacity-0 group-hover/h:opacity-100 focus-visible:opacity-100' : ''}`}
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
    const faces = roleFaces(teams.data, c.team);
    const roles = Object.keys(faces);
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
        aside={c.lastMessage && rowTime(c.lastMessage.createdAt)}
        preview={c.lastMessage ? oneLine(c.lastMessage) : 'No messages yet'}
      />
    );
  }

  return (
    <section aria-label={project.name} className="flex flex-col gap-0.5">
      <div className="group/h flex items-center gap-2 pt-3.5 pr-1 pb-1 pl-2.5">
        <h4 className="text-xs text-muted">Features</h4>
        <AddButton label="New feature" onClick={() => setAddingFeature(true)} />
      </div>
      {groups.length === 0 && <p className="px-2.5 pb-1 text-[13px] text-faint">No features yet</p>}
      <ul className="flex flex-col gap-0.5">
        {groups.map(({ feature: f, channels: cs }) => (
          <li key={f.id}>
            <div
              className={`group/h flex h-8 items-center gap-2 rounded-lg pr-1 pl-2.5 ${pathname === `/features/${f.id}` ? 'bg-active' : 'hover:bg-hover/50'}`}
            >
              <Link
                to={`/features/${f.id}`}
                className="flex min-w-0 flex-1 items-center gap-2 text-[13px] text-soft hover:text-fg"
              >
                <span className="truncate">{f.name}</span>
                <span className="flex-none rounded-pill border border-line px-1.5 text-[10.5px] leading-4 text-faint capitalize">
                  {f.stage}
                </span>
              </Link>
              <AddButton
                reveal
                label={`New channel in ${f.name}`}
                onClick={() => setChannelFeature(f)}
              />
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
          <h4 className="px-2.5 pt-3.5 pb-1 text-xs text-muted">Quick sessions</h4>
          <ul className="flex flex-col gap-0.5">
            {quick.map((c) => (
              <li key={c.id}>{channelRow(c, c.name)}</li>
            ))}
          </ul>
        </>
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
