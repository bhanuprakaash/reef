import { useDecideApproval } from '@/modules/channels/api/decide-approval';
import { useApprovals } from '@/modules/channels/api/get-approvals';
import { buttonClass, errorClass } from '@/utils/classes';

export function ApprovalRows({ channelId }: { channelId: number }) {
  const approvals = useApprovals(channelId);
  const decide = useDecideApproval(channelId);

  if (!approvals.data?.length) return null;

  return (
    <div className="space-y-2">
      <ul className="space-y-2">
        {approvals.data.map((a) => (
          <li
            key={a.id}
            className="flex items-center justify-between gap-3 rounded border border-neutral-300 p-3 text-sm dark:border-neutral-700"
          >
            <p className="whitespace-pre-wrap">
              <span className="font-medium">
                {a.from} to {a.to}:
              </span>{' '}
              {a.text}
            </p>
            <div className="flex gap-2">
              <button
                type="button"
                className={buttonClass}
                disabled={decide.isPending}
                onClick={() => decide.mutate({ id: a.id, approve: true })}
              >
                Approve
              </button>
              <button
                type="button"
                className={buttonClass}
                disabled={decide.isPending}
                onClick={() => decide.mutate({ id: a.id, approve: false })}
              >
                Reject
              </button>
            </div>
          </li>
        ))}
      </ul>
      {decide.isError && <p className={errorClass}>{decide.error.message}</p>}
    </div>
  );
}
