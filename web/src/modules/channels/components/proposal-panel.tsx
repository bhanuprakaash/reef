import { useEffect, useRef } from 'react';
import { useDecideProposal } from '@/modules/channels/api/decide-proposal';
import { useProposal } from '@/modules/channels/api/get-proposal';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';

export function ProposalPanel({
  channelId,
  featureId,
  faces,
}: {
  channelId: number;
  featureId: number | null;
  faces: Record<string, string | undefined>;
}) {
  const proposal = useProposal(channelId);
  const decide = useDecideProposal(channelId, featureId);
  const panel = useRef<HTMLElement>(null);
  const proposalId = proposal.data?.id;

  useEffect(() => {
    if (proposalId !== undefined) panel.current?.scrollIntoView({ block: 'nearest' });
  }, [proposalId]);

  if (proposal.isError) return <p className="text-sm text-danger">{proposal.error.message}</p>;

  const p = proposal.data;
  if (!p) {
    if (decide.data?.status === 'approved') {
      return (
        <p className="text-sm text-done">
          approved: {decide.data.decisionsAdded ?? 0} decisions added
        </p>
      );
    }
    if (decide.data?.status === 'rejected') return <p className="text-sm text-muted">rejected</p>;
    return null;
  }

  const summary = [
    'brief',
    p.notes && 'role notes',
    p.decisions.length > 0 &&
      `${p.decisions.length} ${p.decisions.length === 1 ? 'decision' : 'decisions'}`,
  ]
    .filter(Boolean)
    .join(' · ');

  return (
    <section
      ref={panel}
      aria-label="Proposal"
      className="flex scroll-mb-44 flex-col gap-2.5 rounded-card border border-line bg-card p-4 [--creature-ring:var(--color-card)]"
    >
      <div className="flex items-center gap-2">
        <Creature role={p.role} face={faces[p.role]} size={22} />
        <h2>Wrap-up proposal from {p.role}</h2>
        <span className="ml-auto text-[12.5px] text-muted">{summary}</span>
      </div>
      <div className="rounded-xl bg-field px-3 py-2.5 text-[13.5px] text-soft">
        <h3 className="text-xs font-medium text-muted uppercase">Brief</h3>
        <pre className="font-sans whitespace-pre-wrap">{p.brief}</pre>
      </div>
      {p.notes && (
        <div className="rounded-xl bg-field px-3 py-2.5 text-[13.5px] text-soft">
          <h3 className="text-xs font-medium text-muted uppercase">Notes</h3>
          <pre className="font-sans whitespace-pre-wrap">{p.notes}</pre>
        </div>
      )}
      {p.decisions.length > 0 && (
        <div className="rounded-xl bg-field px-3 py-2.5 text-[13.5px] text-soft">
          <h3 className="text-xs font-medium text-muted uppercase">Decisions</h3>
          <ul className="space-y-2">
            {p.decisions.map((d, i) => (
              <li key={i}>
                <p>{d.body}</p>
                {d.why && <p className="text-sm text-muted">Why: {d.why}</p>}
              </li>
            ))}
          </ul>
        </div>
      )}
      {decide.isError && <p className="text-sm text-danger">{decide.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          disabled={decide.isPending}
          onClick={() => decide.mutate({ proposal: p, action: 'reject' })}
        >
          Reject
        </Button>
        <Button
          variant="primary"
          type="button"
          disabled={decide.isPending}
          onClick={() => decide.mutate({ proposal: p, action: 'approve' })}
        >
          Approve
        </Button>
      </div>
    </section>
  );
}
