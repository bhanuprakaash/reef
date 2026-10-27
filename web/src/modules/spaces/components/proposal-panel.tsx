import { useEffect, useRef } from 'react';
import { useDecideProposal } from '@/modules/spaces/api/decide-proposal';
import { useProposal } from '@/modules/spaces/api/get-proposal';
import { ProposalBody } from '@/modules/spaces/components/proposal-body';
import { Button } from '@/components/button';
import { Creature, type Faces } from '@/components/creature';

export function ProposalPanel({
  spaceId,
  featureId,
  faces,
}: {
  spaceId: number;
  featureId: number | null;
  faces: Faces;
}) {
  const proposal = useProposal(spaceId);
  const decide = useDecideProposal(spaceId, featureId);
  const panel = useRef<HTMLElement>(null);
  const proposalId = proposal.data?.id;

  useEffect(() => {
    if (proposalId !== undefined) panel.current?.scrollIntoView({ block: 'nearest' });
  }, [proposalId]);

  if (proposal.isError) return <p className="text-sm text-danger">{proposal.error.message}</p>;

  const p = proposal.data;
  if (!p) {
    if (decide.data?.status === 'approved') {
      if (decide.data.kind === 'brief')
        return <p className="text-sm text-done">approved: brief saved</p>;
      return (
        <div className="space-y-1">
          <p className="text-sm text-done">approved: {decide.data.itemsAdded ?? 0} items saved</p>
          {decide.data.warnings?.map((w, i) => (
            <p key={i} className="text-sm text-muted">
              {w}
            </p>
          ))}
        </div>
      );
    }
    if (decide.data?.status === 'rejected') return <p className="text-sm text-muted">rejected</p>;
    return null;
  }

  const summary = [
    'brief',
    p.items.length > 0 && `${p.items.length} ${p.items.length === 1 ? 'item' : 'items'}`,
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
        <Creature look={faces[p.role]} size={22} />
        <h2>{p.kind === 'brief' ? 'Brief update' : `Wrap-up proposal from ${p.role}`}</h2>
        <span className="ml-auto text-[12.5px] text-muted">{summary}</span>
      </div>
      <ProposalBody p={p} />
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
