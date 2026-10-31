import { Link } from 'react-router';
import { Button, buttonClass } from '@/components/button';
import { Creature, faceOf } from '@/components/creature';
import { useContinueSpace } from '@/lib/api/continue-space';
import { useRetryTurn } from '@/lib/api/retry-turn';
import { useBots } from '@/lib/api/get-bots';
import { useDismissItem } from '@/modules/inbox/api/dismiss-item';
import { age } from '@/modules/inbox/utils/age';
import { kindLabels } from '@/modules/inbox/utils/new-items';
import type { SpacePreview, InboxItem } from '@/types/api';
import { itemPath } from '@/utils/anchors';
import { memberFaces } from '@/utils/role-faces';

const kindColors: Record<InboxItem['kind'], string> = {
  paused: 'text-failed',
  held: 'text-failed',
  failed: 'text-failed',
  review: 'text-paused',
  waiting: 'text-working',
  sub_item: 'text-muted',
  proposed: 'text-muted',
  started: 'text-muted',
  space_proposed: 'text-muted',
};

export function InboxRow({ item, space }: { item: InboxItem; space?: SpacePreview }) {
  const cont = useContinueSpace();
  const retry = useRetryTurn();
  const dismiss = useDismissItem();
  const bots = useBots();
  const pending = cont.isPending || retry.isPending || dismiss.isPending;
  const error = cont.error ?? retry.error ?? dismiss.error;
  const { spaceId, turnId } = item;
  const primary = item.kind === 'review' || item.kind === 'waiting';
  const where =
    item.spaceId === 0 ? `Work item ${item.workItemId}` : `#${space?.name ?? `Space ${spaceId}`}`;

  return (
    <li className="flex gap-4 rounded-card border border-line bg-card px-5 py-[18px]">
      <Creature look={faceOf(memberFaces(bots.data, space?.members), space?.botId)} size={40} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-2 text-xs">
          <span className={`uppercase tracking-wider ${kindColors[item.kind]}`}>
            {kindLabels[item.kind]}
          </span>
          <span className="text-muted">
            {where} · {age(item.at)}
          </span>
        </div>
        <div className="mt-1 whitespace-pre-wrap text-[15px]">{item.text}</div>
        <div className="mt-3.5 flex flex-wrap items-center gap-2">
          {(item.kind === 'paused' || item.kind === 'held') && (
            <Button
              type="button"
              variant="primary"
              disabled={pending}
              onClick={() => cont.mutate(spaceId)}
            >
              Continue
            </Button>
          )}
          {item.kind === 'failed' && turnId !== undefined && (
            <Button
              type="button"
              variant="primary"
              disabled={pending}
              onClick={() => retry.mutate(turnId)}
            >
              Retry
            </Button>
          )}
          <Link to={itemPath(item)} className={buttonClass(primary ? 'primary' : 'ghost')}>
            {item.group === 'needs_you' ? 'Open Space' : item.kind === 'review' ? 'Review' : 'Open'}
          </Link>
          {item.group === 'fyi' && (
            <Button
              type="button"
              variant="ghost"
              disabled={pending}
              onClick={() => dismiss.mutate(item)}
            >
              Dismiss
            </Button>
          )}
        </div>
        {error && <p className="mt-2 text-sm text-danger">{error.message}</p>}
      </div>
    </li>
  );
}
