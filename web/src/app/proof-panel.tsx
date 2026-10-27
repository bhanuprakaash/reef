import { Link } from 'react-router';
import { useWorkItemPR } from '@/modules/spaces/api/get-work-item-pr';
import { useProof } from '@/modules/work-items/api/get-proof';
import { Card } from '@/components/card';
import { EmptyState } from '@/components/empty-state';
import { StageChip } from '@/components/stage-chip';
import type { WorkItemPR } from '@/types/api';

const PR_COLOR: Record<WorkItemPR['state'], string> = {
  OPEN: 'var(--color-done)',
  MERGED: 'var(--color-architect)',
  CLOSED: 'var(--color-failed)',
};

const prState = (pr: WorkItemPR) =>
  pr.isDraft && pr.state === 'OPEN' ? 'Draft' : pr.state[0] + pr.state.slice(1).toLowerCase();

export function ProofPanel({ workItemId }: { workItemId: number }) {
  const proof = useProof(workItemId);
  const p = proof.data;

  return (
    <Card
      title="Proof"
      aside={
        p && (
          <span className="text-xs text-muted">
            Submitted {new Date(p.submittedAt).toLocaleString()} from{' '}
            <Link to={`/spaces/${p.spaceId}`} className="border-b border-line text-soft">
              Space {p.spaceId}
            </Link>
          </span>
        )
      }
    >
      {proof.isError && <p className="text-sm text-danger">{proof.error.message}</p>}
      {proof.data === null && (
        <EmptyState
          title="Not submitted yet"
          body="The lead's commands and screenshots land here when it submits for review."
          face="shark"
          compact
        />
      )}
      {p && (
        <div className="space-y-4">
          <ProofPullRequest workItemId={workItemId} />
          {p.commands.length === 0 && p.screenshots.length === 0 && (
            <p className="text-sm text-muted">Submitted with no commands or screenshots.</p>
          )}
          {p.commands.length > 0 && (
            <ul className="space-y-3">
              {p.commands.map((c, i) => (
                <li key={i}>
                  <code className="block font-mono text-[13px] break-all text-soft">
                    $ {c.command}
                  </code>
                  <pre className="mt-1 max-h-48 overflow-auto rounded-field bg-main p-3 font-mono text-xs whitespace-pre-wrap text-muted">
                    {c.output}
                  </pre>
                </li>
              ))}
            </ul>
          )}
          {p.screenshots.length > 0 && (
            <ul className="space-y-3">
              {p.screenshots.map((s, i) => (
                <li key={s}>
                  <img
                    src={`/work-items/${workItemId}/proof/screenshots/${i}`}
                    alt={`Screenshot ${i + 1}`}
                    className="max-h-96 max-w-full rounded-field border border-line"
                  />
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </Card>
  );
}

function ProofPullRequest({ workItemId }: { workItemId: number }) {
  const query = useWorkItemPR(workItemId);
  const pr = query.data?.pr;
  if (query.isError) return <p className="text-sm text-danger">{query.error.message}</p>;
  if (!query.data) return null;
  if (!pr) return <p className="text-[13px] text-muted">No pull request for this Work item yet.</p>;
  return (
    <p className="flex items-center gap-2 text-[13px] text-muted">
      <StageChip label={prState(pr)} color={PR_COLOR[pr.state]} />
      <a
        href={pr.url}
        target="_blank"
        rel="noreferrer"
        className="min-w-0 truncate border-b border-line text-soft"
      >
        #{pr.number} {pr.title}
      </a>
    </p>
  );
}
