import { Link } from 'react-router';
import { Button, buttonClass } from '@/components/button';
import { Creature } from '@/components/creature';
import { useContinueChannel } from '@/lib/api/continue-channel';
import { useDecideApproval } from '@/lib/api/decide-approval';
import { useRetryTurn } from '@/lib/api/retry-turn';
import { useTeams } from '@/lib/api/get-teams';
import { useDismissWaiting } from '@/modules/inbox/api/dismiss-waiting';
import { age } from '@/modules/inbox/utils/age';
import { kindLabels } from '@/modules/inbox/utils/new-items';
import type { Channel, InboxItem } from '@/types/api';
import { itemPath } from '@/utils/anchors';
import { roleFaces } from '@/utils/role-faces';

const kindColors: Record<InboxItem['kind'], string> = {
  approval: 'text-paused',
  paused: 'text-failed',
  failed: 'text-failed',
  waiting: 'text-working',
};

export function InboxRow({ item, channel }: { item: InboxItem; channel?: Channel }) {
  const decide = useDecideApproval();
  const cont = useContinueChannel();
  const retry = useRetryTurn();
  const dismiss = useDismissWaiting();
  const teams = useTeams();
  const pending = decide.isPending || cont.isPending || retry.isPending || dismiss.isPending;
  const error = decide.error ?? cont.error ?? retry.error ?? dismiss.error;
  const { channelId, approvalId, turnId, messageId } = item;

  return (
    <li className="flex gap-4 rounded-card border border-line bg-card px-5 py-[18px]">
      <Creature
        role={channel?.role ?? ''}
        face={roleFaces(teams.data, channel?.team)[channel?.role ?? '']}
        size={40}
      />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-2 text-xs">
          <span className={`uppercase tracking-wider ${kindColors[item.kind]}`}>
            {kindLabels[item.kind]}
          </span>
          <span className="text-muted">
            #{channel?.name ?? `Channel ${channelId}`} · {age(item.at)}
          </span>
        </div>
        <div className="mt-1 whitespace-pre-wrap text-[15px]">{item.text}</div>
        <div className="mt-3.5 flex flex-wrap items-center gap-2">
          {item.kind === 'approval' && approvalId !== undefined && (
            <>
              <Button
                type="button"
                variant="primary"
                disabled={pending}
                onClick={() => decide.mutate({ channelId, id: approvalId, approve: true })}
              >
                Approve
              </Button>
              <Button
                type="button"
                disabled={pending}
                onClick={() => decide.mutate({ channelId, id: approvalId, approve: false })}
              >
                Reject
              </Button>
            </>
          )}
          {item.kind === 'paused' && (
            <Button
              type="button"
              variant="primary"
              disabled={pending}
              onClick={() => cont.mutate(channelId)}
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
          <Link
            to={itemPath(item)}
            className={buttonClass(item.kind === 'waiting' ? 'primary' : 'ghost')}
          >
            {item.kind === 'waiting' ? 'Open' : 'Open channel'}
          </Link>
          {item.kind === 'waiting' && messageId !== undefined && (
            <Button
              type="button"
              variant="ghost"
              disabled={pending}
              onClick={() => dismiss.mutate({ channelId, messageId })}
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
