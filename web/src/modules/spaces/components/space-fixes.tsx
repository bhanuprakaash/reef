import {
  type RepoRequest,
  useAbandonSpaceWorkspace,
  useSpaceWorkspaces,
} from '@/modules/spaces/api/space-workspaces';
import { Button } from '@/components/button';

const label = (r: RepoRequest) =>
  `${r.path.split('/').pop()} ${r.provider === 'github' ? '#' : '!'}${r.number}`;

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

export function SpaceFixes({ spaceId }: { spaceId: number }) {
  const query = useSpaceWorkspaces(spaceId);
  const abandon = useAbandonSpaceWorkspace(spaceId);
  const fixes = (query.data ?? []).filter((w) => w.state !== 'removed');
  if (fixes.length === 0 && !query.isError) return null;

  return (
    <div>
      <h2 className="mb-1.5 text-[13px] text-muted">Fixes</h2>
      {query.isError && <p className="text-sm text-danger">{query.error.message}</p>}
      <ul>
        {fixes.map((w) => {
          const group = fixes.filter((f) => f.name === w.name).flatMap((f) => f.requests);
          const steps = mergeSteps(group);
          const ordered = group.some((r) => r.dependsOn.length > 0);
          return (
            <li key={w.id} className="py-1.5 text-[14.5px]">
              <div className="flex min-h-7 items-center gap-2">
                <span className="min-w-0 flex-1 truncate">
                  {w.name} in {w.projectName}
                </span>
                <span className="text-[13px] text-muted">
                  {w.state === 'ready' && w.dirty ? 'uncommitted changes' : w.state}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={`Abandon ${w.name}`}
                  disabled={abandon.isPending}
                  onClick={() => abandon.mutate(w.id)}
                >
                  Abandon
                </Button>
              </div>
              <p className="truncate text-[13px] text-muted">
                {w.branch}
                {w.head !== '' && ` at ${w.head}`}
              </p>
              {w.note !== '' && <p className="text-[13px] text-muted">{w.note}</p>}
              {w.requests.map((r) => (
                <div key={r.id} className="text-[13px] text-muted">
                  <p className="flex items-center gap-2">
                    <a
                      href={r.url}
                      target="_blank"
                      rel="noreferrer"
                      title={r.path}
                      className="min-w-0 truncate border-b border-line text-soft"
                    >
                      {label(r)}
                    </a>
                    <span>{r.state}</span>
                  </p>
                  {ordered && (
                    <p className="truncate">
                      merge step {steps.get(r.id)}
                      {r.dependsOn.length > 0 &&
                        `, after ${r.dependsOn
                          .flatMap((id) => group.find((g) => g.id === id) ?? [])
                          .map(label)
                          .join(', ')}`}
                    </p>
                  )}
                </div>
              ))}
            </li>
          );
        })}
      </ul>
      {abandon.isError && <p className="text-sm text-danger">{abandon.error.message}</p>}
    </div>
  );
}
