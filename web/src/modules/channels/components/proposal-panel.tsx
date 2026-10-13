import { useEffect, useRef } from 'react';
import { useDecideProposal } from '@/modules/channels/api/decide-proposal';
import { useProposal } from '@/modules/channels/api/get-proposal';
import { buttonClass, errorClass, mutedClass } from '@/utils/classes';

export function ProposalPanel({
  channelId,
  featureId,
}: {
  channelId: number;
  featureId: number | null;
}) {
  const proposal = useProposal(channelId);
  const decide = useDecideProposal(channelId, featureId);
  const panel = useRef<HTMLElement>(null);
  const proposalId = proposal.data?.id;

  useEffect(() => {
    if (proposalId !== undefined) panel.current?.scrollIntoView({ block: 'nearest' });
  }, [proposalId]);

  if (proposal.isError) return <p className={errorClass}>{proposal.error.message}</p>;

  const p = proposal.data;
  if (!p) {
    if (decide.data?.status === 'approved') {
      return (
        <p className="text-sm text-green-700 dark:text-green-400">
          approved: {decide.data.decisionsAdded ?? 0} decisions added
        </p>
      );
    }
    if (decide.data?.status === 'rejected') return <p className={mutedClass}>rejected</p>;
    return null;
  }

  return (
    <section
      ref={panel}
      aria-label="Proposal"
      className="scroll-mb-44 space-y-3 rounded border border-neutral-300 p-3 dark:border-neutral-700"
    >
      <h2 className="text-sm font-medium">Proposal from {p.role}</h2>
      <div>
        <h3 className="text-xs font-medium uppercase text-neutral-500">Brief</h3>
        <pre className="font-sans text-sm whitespace-pre-wrap">{p.brief}</pre>
      </div>
      {p.notes && (
        <div>
          <h3 className="text-xs font-medium uppercase text-neutral-500">Notes</h3>
          <pre className="font-sans text-sm whitespace-pre-wrap">{p.notes}</pre>
        </div>
      )}
      {p.decisions.length > 0 && (
        <div>
          <h3 className="text-xs font-medium uppercase text-neutral-500">Decisions</h3>
          <ul className="space-y-2 text-sm">
            {p.decisions.map((d, i) => (
              <li key={i}>
                <p>{d.body}</p>
                {d.why && <p className={mutedClass}>Why: {d.why}</p>}
              </li>
            ))}
          </ul>
        </div>
      )}
      {decide.isError && <p className={errorClass}>{decide.error.message}</p>}
      <div className="flex gap-2">
        <button
          type="button"
          className={buttonClass}
          disabled={decide.isPending}
          onClick={() => decide.mutate({ proposal: p, action: 'approve' })}
        >
          Approve
        </button>
        <button
          type="button"
          className={buttonClass}
          disabled={decide.isPending}
          onClick={() => decide.mutate({ proposal: p, action: 'reject' })}
        >
          Reject
        </button>
      </div>
    </section>
  );
}
