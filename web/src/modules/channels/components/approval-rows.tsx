import { useDecideApproval } from '@/lib/api/decide-approval';
import { useApprovals } from '@/modules/channels/api/get-approvals';
import { approvalAnchor } from '@/utils/anchors';
import { Button } from '@/components/button';

export function ApprovalRows({ channelId }: { channelId: number }) {
  const approvals = useApprovals(channelId);
  const decide = useDecideApproval();

  if (!approvals.data?.length) return null;

  return (
    <div className="space-y-2">
      <ul className="space-y-2">
        {approvals.data.map((a) => (
          <li
            key={a.id}
            id={approvalAnchor(a.id)}
            className="flex items-center justify-between gap-3 rounded border border-line p-3 text-sm"
          >
            <p className="whitespace-pre-wrap">
              <span className="font-medium">
                {a.from} → {a.to}:
              </span>{' '}
              {a.text}
            </p>
            <div className="flex gap-2">
              <Button
                variant="primary"
                type="button"
                disabled={decide.isPending}
                onClick={() => decide.mutate({ channelId, id: a.id, approve: true })}
              >
                Approve
              </Button>
              <Button
                variant="destructive"
                type="button"
                disabled={decide.isPending}
                onClick={() => decide.mutate({ channelId, id: a.id, approve: false })}
              >
                Reject
              </Button>
            </div>
          </li>
        ))}
      </ul>
      {decide.isError && <p className="text-sm text-danger">{decide.error.message}</p>}
    </div>
  );
}
