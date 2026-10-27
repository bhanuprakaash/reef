import { Link } from 'react-router';
import { useProof } from '@/modules/work-items/api/get-proof';
import { Card } from '@/components/card';
import { EmptyState } from '@/components/empty-state';

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
            <ul className="space-y-1">
              {p.screenshots.map((s) => (
                <li key={s} className="font-mono text-xs break-all text-muted">
                  {s}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </Card>
  );
}
