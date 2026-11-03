import { StageChip } from '@/components/stage-chip';
import { useRepoRequests } from '@/modules/spaces/api/get-repo-requests';
import type { RepoRequest } from '@/types/api';

const STATE: Record<RepoRequest['state'], { label: string; color: string }> = {
  opened: { label: 'Open', color: 'var(--color-done)' },
  merged: { label: 'Merged', color: 'var(--color-architect)' },
  closed: { label: 'Closed, not merged', color: 'var(--color-failed)' },
};

const REVIEW: Record<string, string> = {
  approved: 'approved',
  changes_requested: 'changes requested',
  review_required: 'review required',
};

const label = (r: RepoRequest) => `${r.path}${r.provider === 'gitlab' ? '!' : '#'}${r.number}`;

// The position of each request in the order to merge them: after every request it depends on.
// Reef never merges; the user does.
function mergeSteps(requests: RepoRequest[]) {
  const byId = new Map(requests.map((r) => [r.id, r]));
  const steps = new Map<number, number>();
  const step = (r: RepoRequest, path: number[]): number => {
    const known = steps.get(r.id);
    if (known !== undefined) return known;
    if (path.includes(r.id)) return 1;
    const deps = r.dependsOn.flatMap((id) => byId.get(id) ?? []);
    const n = 1 + Math.max(0, ...deps.map((d) => step(d, [...path, r.id])));
    steps.set(r.id, n);
    return n;
  };
  requests.forEach((r) => step(r, []));
  return steps;
}

// The pull or merge requests of one workspace, or all of them when there is no workspaceId.
// `path` lists every request of a Work item or Space, so a merge order can name the requests it
// points at.
export function RequestList({
  path,
  workspaceId,
  empty,
}: {
  path: string;
  workspaceId?: number;
  empty?: string;
}) {
  const query = useRepoRequests(path);
  const all = query.data ?? [];
  const steps = mergeSteps(all);
  const mine = all.filter((r) => workspaceId === undefined || r.workspaceId === workspaceId);
  if (mine.length === 0) return empty ? <span>{empty}</span> : null;

  return (
    <ul className="space-y-1">
      {mine.map((r) => {
        const failing = r.checks.filter((c) => c.state === 'fail').map((c) => c.name);
        const after = r.dependsOn.flatMap((id) => all.filter((o) => o.id === id).map(label));
        const ordered = after.length > 0 || all.some((o) => o.dependsOn.includes(r.id));
        return (
          <li key={r.id} className="space-y-0.5 text-[13px]">
            <div className="flex flex-wrap items-center gap-2">
              <StageChip label={STATE[r.state].label} color={STATE[r.state].color} />
              <a
                href={r.url}
                target="_blank"
                rel="noreferrer"
                className="min-w-0 truncate border-b border-line text-soft"
              >
                {label(r)}
              </a>
              {r.review && <span className="text-muted">{REVIEW[r.review] ?? r.review}</span>}
              {failing.length > 0 && (
                <span className="text-failed">failing: {failing.join(', ')}</span>
              )}
            </div>
            {ordered && (
              <p className="truncate text-muted">
                merge step {steps.get(r.id)}
                {after.length > 0 && `, after ${after.join(', ')}`}
              </p>
            )}
            {r.checkError !== '' && (
              <p className="text-danger">Status not read: {r.checkError}. Retrying.</p>
            )}
          </li>
        );
      })}
    </ul>
  );
}
