import { Link } from 'react-router';
import { Button, buttonClass } from '@/components/button';
import { useContinueChannel } from '@/lib/api/continue-channel';
import { useDecideApproval } from '@/lib/api/decide-approval';
import { useRetryTurn } from '@/lib/api/retry-turn';
import { kindLabels } from '@/modules/inbox/utils/new-items';
import type { InboxItem } from '@/types/api';
import { itemPath } from '@/utils/anchors';

export function InboxRow({ item, channelName }: { item: InboxItem; channelName: string }) {
  const decide = useDecideApproval();
  const cont = useContinueChannel();
  const retry = useRetryTurn();
  const pending = decide.isPending || cont.isPending || retry.isPending;
  const error = decide.error ?? cont.error ?? retry.error;
  const { channelId, approvalId, turnId } = item;

  return (
    <li className="rounded border border-line p-3 text-sm">
      <div className="flex items-center justify-between gap-3">
        <Link to={itemPath(item)} className="min-w-0 flex-1 hover:underline">
          <span className="font-medium">
            {kindLabels[item.kind]}: {channelName}
          </span>{' '}
          <span className="text-sm text-muted">{new Date(item.at).toLocaleString()}</span>
          <span className="block whitespace-pre-wrap">{item.text}</span>
        </Link>
        <div className="flex gap-2">
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
                variant="destructive"
                disabled={pending}
                onClick={() => decide.mutate({ channelId, id: approvalId, approve: false })}
              >
                Reject
              </Button>
            </>
          )}
          {item.kind === 'paused' && (
            <Button type="button" disabled={pending} onClick={() => cont.mutate(channelId)}>
              Continue
            </Button>
          )}
          {item.kind === 'failed' && turnId !== undefined && (
            <Button type="button" disabled={pending} onClick={() => retry.mutate(turnId)}>
              Retry
            </Button>
          )}
          {item.kind === 'waiting' && (
            <Link to={itemPath(item)} className={buttonClass()}>
              Open
            </Link>
          )}
        </div>
      </div>
      {error && <p className="text-sm text-danger">{error.message}</p>}
    </li>
  );
}
