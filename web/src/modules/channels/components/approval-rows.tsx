import { useDecideApproval } from '@/lib/api/decide-approval';
import { useApprovals } from '@/modules/channels/api/get-approvals';
import { approvalAnchor } from '@/utils/anchors';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';

export function ApprovalRows({
  channelId,
  faces,
}: {
  channelId: number;
  faces: Record<string, string | undefined>;
}) {
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
            className="flex flex-wrap items-center gap-3 rounded-card border border-line bg-card px-4 py-3.5 [--creature-ring:var(--color-card)]"
          >
            <span className="flex items-center gap-1">
              <Creature role={a.from} face={faces[a.from]} size={26} />
              <svg
                width="16"
                height="12"
                viewBox="0 0 16 12"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
                className="text-muted"
              >
                <path d="M1 6h12M9 2l4 4-4 4" />
              </svg>
              <Creature role={a.to} face={faces[a.to]} state="needs-you" size={26} />
            </span>
            <div className="min-w-0 flex-1 basis-48">
              <p className="text-[14.5px] whitespace-pre-wrap">{a.text}</p>
              <p className="text-[13px] text-muted capitalize">{`${a.from} → ${a.to}`}</p>
            </div>
            <Button
              type="button"
              disabled={decide.isPending}
              onClick={() => decide.mutate({ channelId, id: a.id, approve: false })}
            >
              Reject
            </Button>
            <Button
              variant="primary"
              type="button"
              disabled={decide.isPending}
              onClick={() => decide.mutate({ channelId, id: a.id, approve: true })}
            >
              Approve
            </Button>
          </li>
        ))}
      </ul>
      {decide.isError && <p className="text-sm text-danger">{decide.error.message}</p>}
    </div>
  );
}
