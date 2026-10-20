import { Creature, type CreatureState } from '@/components/creature';
import { Skeleton } from '@/components/skeleton';
import { StatusDot, type Status } from '@/components/status-dot';
import { useDiff } from '@/modules/features/api/get-diff';
import { diffCounts } from '@/modules/features/utils/diff-counts';
import type { ChannelPreview, Feature, InboxItem } from '@/types/api';

export function channelState(
  channel: Pick<ChannelPreview, 'closedAt' | 'paused'>,
  working: string[],
  waiting: InboxItem['kind'][],
): { status: Status; label: string } {
  if (channel.closedAt !== null) return { status: 'done', label: 'Wrapped' };
  if (channel.paused !== '') return { status: 'paused', label: `Paused: ${channel.paused}` };
  if (waiting.includes('approval')) {
    return { status: 'needs-you', label: 'Waiting for your approval' };
  }
  if (waiting.includes('failed')) return { status: 'failed', label: 'A turn failed' };
  if (working.length > 0) return { status: 'working', label: 'Working' };
  if (waiting.includes('waiting')) return { status: 'needs-you', label: 'Waiting for you' };
  return { status: 'asleep', label: 'Idle' };
}

function Changes({ featureId }: { featureId: number }) {
  const diff = useDiff(featureId);
  if (diff.isError) return <span className="text-danger">Unavailable</span>;
  if (!diff.data) return <Skeleton className="h-4 w-24" />;
  if (diff.data.length === 0) return <span className="text-muted">No changes yet</span>;
  const { added, removed } = diffCounts(diff.data);
  return (
    <span>
      {diff.data.length} file{diff.data.length === 1 ? '' : 's'}{' '}
      <span className="text-done">+{added}</span> <span className="text-failed">−{removed}</span>
    </span>
  );
}

export function ChannelCard({
  channel,
  feature,
  faces,
  working,
  waiting,
}: {
  channel: ChannelPreview;
  feature: Feature | undefined;
  faces: Record<string, string | undefined>;
  working: string[];
  waiting: InboxItem['kind'][];
}) {
  const state = channelState(channel, working, waiting);
  const roles = Object.keys(faces);
  const team = [...roles, ...working.filter((r) => !roles.includes(r))];
  return (
    <div className="flex flex-col gap-3">
      <div>
        <div className="truncate">#{channel.name}</div>
        <div className="mt-1 flex items-center gap-2 text-[13px] text-soft">
          <StatusDot status={state.status} />
          <span className="line-clamp-2">{state.label}</span>
        </div>
      </div>
      {feature && (
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-[13px]">
          <dt className="text-muted">Branch</dt>
          <dd className="truncate font-mono text-xs leading-5">feature/{feature.slug}</dd>
          <dt className="text-muted">Changes</dt>
          <dd>
            <Changes featureId={feature.id} />
          </dd>
        </dl>
      )}
      <ul aria-label="Team" className="flex flex-col gap-1.5">
        {(team.length > 0 ? team : [channel.role]).map((role) => {
          const active = working.includes(role);
          const creature: CreatureState = active ? 'working' : 'idle';
          return (
            <li key={role} className="flex items-center gap-2.5">
              <Creature
                role={role}
                face={faces[role]}
                state={creature}
                size={22}
                animate={active}
              />
              <span className="min-w-0 flex-1 truncate text-[13px] capitalize">{role}</span>
              <span className={`text-xs ${active ? 'text-working' : 'text-muted'}`}>
                {active ? 'working' : 'idle'}
              </span>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
