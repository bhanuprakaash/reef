import { Creature, type CreatureState, type Faces } from '@/components/creature';
import { Skeleton } from '@/components/skeleton';
import { StatusDot, type Status } from '@/components/status-dot';
import { useDiff } from '@/modules/work-items/api/get-diff';
import { diffCounts } from '@/modules/work-items/utils/diff-counts';
import type { SpacePreview, WorkItem, InboxItem } from '@/types/api';

export function spaceState(
  space: Pick<SpacePreview, 'closedAt' | 'paused'>,
  working: string[],
  waiting: InboxItem['kind'][],
): { status: Status; label: string } {
  if (space.closedAt !== null) return { status: 'done', label: 'Wrapped' };
  if (space.paused !== '') return { status: 'paused', label: `Paused: ${space.paused}` };
  if (waiting.includes('failed')) return { status: 'failed', label: 'A turn failed' };
  if (working.length > 0) return { status: 'working', label: 'Working' };
  if (waiting.includes('waiting')) return { status: 'needs-you', label: 'Waiting for you' };
  return { status: 'asleep', label: 'Idle' };
}

function Changes({ workItemId }: { workItemId: number }) {
  const diff = useDiff(workItemId);
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

export function SpaceCard({
  space,
  workItem,
  faces,
  working,
  waiting,
}: {
  space: SpacePreview;
  workItem: WorkItem | undefined;
  faces: Faces;
  working: string[];
  waiting: InboxItem['kind'][];
}) {
  const state = spaceState(space, working, waiting);
  const roles = Object.keys(faces);
  const team = [...roles, ...working.filter((r) => !roles.includes(r))];
  return (
    <div className="flex flex-col gap-3">
      <div>
        <div className="truncate">#{space.name}</div>
        <div className="mt-1 flex items-center gap-2 text-[13px] text-soft">
          <StatusDot status={state.status} />
          <span className="line-clamp-2">{state.label}</span>
        </div>
      </div>
      {workItem && (
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-[13px]">
          <dt className="text-muted">Branch</dt>
          <dd className="truncate font-mono text-xs leading-5">work-item/{workItem.id}</dd>
          <dt className="text-muted">Changes</dt>
          <dd>
            <Changes workItemId={workItem.id} />
          </dd>
        </dl>
      )}
      <ul aria-label="Team" className="flex flex-col gap-1.5">
        {(team.length > 0 ? team : [space.role]).map((role) => {
          const active = working.includes(role);
          const creature: CreatureState = active ? 'working' : 'idle';
          return (
            <li key={role} className="flex items-center gap-2.5">
              <Creature look={faces[role]} state={creature} size={22} animate={active} />
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
