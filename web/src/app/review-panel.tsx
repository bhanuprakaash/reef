import { useState, type ReactNode } from 'react';
import { useDecideReview } from '@/modules/work-items/api/decide-review';
import { useProof } from '@/modules/work-items/api/get-proof';
import { useProposal } from '@/modules/spaces/api/get-proposal';
import { ProposalBody } from '@/modules/spaces/components/proposal-body';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { TextArea } from '@/components/text-area';

// ReviewPanel is where the user decides on a top-level item in review: Done approves the
// owner's wrap-up proposal, Send back returns the item to the queue with a note for the owner.
// The user can decide whether or not the item has a proof.
export function ReviewPanel({ workItemId }: { workItemId: number }) {
  const proof = useProof(workItemId);
  if (proof.isPending) return null;
  return (
    <Decision workItemId={workItemId}>
      {proof.isError && <p className="text-sm text-danger">{proof.error.message}</p>}
      {proof.data ? (
        <WrapUp spaceId={proof.data.spaceId} />
      ) : (
        !proof.isError && <p className="text-[13px] text-muted">No proof submitted.</p>
      )}
    </Decision>
  );
}

function WrapUp({ spaceId }: { spaceId: number }) {
  const proposal = useProposal(spaceId);
  const p = proposal.data;
  return (
    <>
      {proposal.isError && <p className="text-sm text-danger">{proposal.error.message}</p>}
      {p ? (
        <>
          <p className="text-[13px] text-muted">
            Wrap-up proposal from {p.role}. Done saves its brief and memory items.
          </p>
          <ProposalBody p={p} />
        </>
      ) : (
        <p className="text-[13px] text-muted">No wrap-up proposal yet.</p>
      )}
    </>
  );
}

function Decision({ workItemId, children }: { workItemId: number; children: ReactNode }) {
  const decide = useDecideReview(workItemId);
  const [note, setNote] = useState('');

  return (
    <Card title="Review">
      <div className="flex flex-col gap-3">
        {children}
        <label className="flex flex-col gap-1.5 text-[13px] text-muted">
          Note for the owner
          <TextArea
            rows={3}
            placeholder="What must change before this is done"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </label>
        {decide.isError && <p className="text-sm text-danger">{decide.error.message}</p>}
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            disabled={decide.isPending || note.trim() === ''}
            onClick={() => decide.mutate({ status: 'queued', note })}
          >
            Send back
          </Button>
          <Button
            type="button"
            variant="primary"
            loading={decide.isPending}
            onClick={() => decide.mutate({ status: 'done' })}
          >
            Done
          </Button>
        </div>
      </div>
    </Card>
  );
}
