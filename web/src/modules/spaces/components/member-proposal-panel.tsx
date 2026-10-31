import { useDecideMemberProposal, useMemberProposals } from '@/modules/spaces/api/member-proposals';
import { Button } from '@/components/button';

export function MemberProposalPanel({ spaceId }: { spaceId: number }) {
  const proposals = useMemberProposals(spaceId);
  const decide = useDecideMemberProposal(spaceId);

  if (!proposals.data?.length) return null;
  return (
    <>
      {proposals.data.map((p) => (
        <section
          key={p.id}
          aria-label="Member proposal"
          className="flex flex-col gap-2.5 rounded-card border border-line bg-card p-4"
        >
          <h2>
            {p.proposedBy} proposes adding {p.botName}
          </h2>
          <p className="text-[13.5px] text-soft">{p.reason}</p>
          {decide.isError && <p className="text-sm text-danger">{decide.error.message}</p>}
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              disabled={decide.isPending}
              onClick={() => decide.mutate({ id: p.id, action: 'reject' })}
            >
              Reject
            </Button>
            <Button
              variant="primary"
              type="button"
              disabled={decide.isPending}
              onClick={() => decide.mutate({ id: p.id, action: 'approve' })}
            >
              Add {p.botName}
            </Button>
          </div>
        </section>
      ))}
    </>
  );
}
