import { Link } from 'react-router';
import { CleanupStatus } from '@/modules/spaces/components/cleanup-status';
import { RequestList } from '@/modules/spaces/components/request-list';
import { useProof } from '@/modules/work-items/api/get-proof';
import { useWorkspaces } from '@/modules/work-items/api/get-workspaces';
import { Card } from '@/components/card';
import { EmptyState } from '@/components/empty-state';
import type { ProofCommand, ProofRepository } from '@/types/api';

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
          body="The owner's commands and screenshots land here when it submits for review."
          face="shark"
          compact
        />
      )}
      {proof.data === null && <RequestList path={`/work-items/${workItemId}/requests`} />}
      {p && (
        <div className="space-y-4">
          {p.repositories.map((r) => (
            <Repository key={r.workspaceId} r={r} workItemId={workItemId} />
          ))}
          {p.commands.length === 0 &&
            p.repositories.every((r) => r.checks.length === 0) &&
            p.screenshots.length === 0 && (
              <p className="text-sm text-muted">Submitted with no commands or screenshots.</p>
            )}
          <Commands commands={p.commands} />
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

function Commands({ commands }: { commands: ProofCommand[] }) {
  if (commands.length === 0) return null;
  return (
    <ul className="space-y-3">
      {commands.map((c, i) => (
        <li key={i}>
          <code className="block font-mono text-[13px] break-all text-soft">$ {c.command}</code>
          <pre className="mt-1 max-h-48 overflow-auto rounded-field bg-main p-3 font-mono text-xs whitespace-pre-wrap text-muted">
            {c.output}
          </pre>
        </li>
      ))}
    </ul>
  );
}

function Repository({ r, workItemId }: { r: ProofRepository; workItemId: number }) {
  const workspace = useWorkspaces(workItemId).data?.find((w) => w.id === r.workspaceId);
  return (
    <section className="space-y-2 rounded-field border border-line p-3">
      <h3 className="text-sm font-medium">{r.projectName}</h3>
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-[13px] text-muted">
        <dt>Branch</dt>
        <dd className="font-mono break-all text-soft">{r.branch}</dd>
        <dt>Commit</dt>
        <dd className="font-mono text-soft">
          {r.head || 'none'}
          {r.dirty && ' (uncommitted changes)'}
        </dd>
        <dt>Base</dt>
        <dd className="font-mono break-all text-soft">{r.baseRef || 'none'}</dd>
        <dt>Workspace</dt>
        <dd className="font-mono break-all text-soft">{r.dir}</dd>
        <dt>Pull request</dt>
        <dd>
          <RequestList
            path={`/work-items/${workItemId}/requests`}
            workspaceId={r.workspaceId}
            empty="Not opened yet"
          />
        </dd>
      </dl>
      <CleanupStatus w={workspace} />
      <Commands commands={r.checks} />
    </section>
  );
}
