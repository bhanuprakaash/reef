import { useState } from 'react';
import { useDecideReview } from '@/modules/features/api/decide-review';
import { useProof } from '@/modules/features/api/get-proof';
import { useProposal } from '@/modules/spaces/api/get-proposal';
import { ProposalBody } from '@/modules/spaces/components/proposal-body';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { TextArea } from '@/components/text-area';

// ReviewPanel is where the user decides on a top-level item in review: Done approves the
// lead's wrap-up proposal, Send back returns the item to the queue with a note for the lead.
export function ReviewPanel({ workItemId }: { workItemId: number }) {
  const proof = useProof(workItemId);
  if (!proof.data) return null;
  return <Decision workItemId={workItemId} spaceId={proof.data.spaceId} />;
}

function Decision({ workItemId, spaceId }: { workItemId: number; spaceId: number }) {
  const proposal = useProposal(spaceId);
  const decide = useDecideReview(workItemId);
  const [note, setNote] = useState('');
  const p = proposal.data;

  return (
    <Card title="Review">
      <div className="flex flex-col gap-3">
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
        <label className="flex flex-col gap-1.5 text-[13px] text-muted">
          Note for the lead
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
