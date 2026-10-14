import { Link } from 'react-router';
import { useContinueChannel } from '@/lib/api/continue-channel';
import { useDecideApproval } from '@/lib/api/decide-approval';
import { useRetryTurn } from '@/lib/api/retry-turn';
import { kindLabels } from '@/modules/inbox/utils/new-items';
import type { InboxItem } from '@/types/api';
import { itemPath } from '@/utils/anchors';
import { buttonClass, errorClass, mutedClass } from '@/utils/classes';

export function InboxRow({ item, channelName }: { item: InboxItem; channelName: string }) {
  const decide = useDecideApproval();
  const cont = useContinueChannel();
  const retry = useRetryTurn();
  const pending = decide.isPending || cont.isPending || retry.isPending;
  const error = decide.error ?? cont.error ?? retry.error;
  const { channelId, approvalId, turnId } = item;

  return (
    <li className="rounded border border-neutral-300 p-3 text-sm dark:border-neutral-700">
      <div className="flex items-center justify-between gap-3">
        <Link to={itemPath(item)} className="min-w-0 flex-1 hover:underline">
          <span className="font-medium">
            {kindLabels[item.kind]}: {channelName}
          </span>{' '}
          <span className={mutedClass}>{new Date(item.at).toLocaleString()}</span>
          <span className="block whitespace-pre-wrap">{item.text}</span>
        </Link>
        <div className="flex gap-2">
          {item.kind === 'approval' && approvalId !== undefined && (
            <>
              <button
                type="button"
                className={buttonClass}
                disabled={pending}
                onClick={() => decide.mutate({ channelId, id: approvalId, approve: true })}
              >
                Approve
              </button>
              <button
                type="button"
                className={buttonClass}
                disabled={pending}
                onClick={() => decide.mutate({ channelId, id: approvalId, approve: false })}
              >
                Reject
              </button>
            </>
          )}
          {item.kind === 'paused' && (
            <button
              type="button"
              className={buttonClass}
              disabled={pending}
              onClick={() => cont.mutate(channelId)}
            >
              Continue
            </button>
          )}
          {item.kind === 'failed' && turnId !== undefined && (
            <button
              type="button"
              className={buttonClass}
              disabled={pending}
              onClick={() => retry.mutate(turnId)}
            >
              Retry
            </button>
          )}
          {item.kind === 'waiting' && (
            <Link to={itemPath(item)} className={buttonClass}>
              Open
            </Link>
          )}
        </div>
      </div>
      {error && <p className={errorClass}>{error.message}</p>}
    </li>
  );
}
