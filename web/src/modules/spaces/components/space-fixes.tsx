import {
  useAbandonSpaceWorkspace,
  useSpaceWorkspaces,
} from '@/modules/spaces/api/space-workspaces';
import { Button } from '@/components/button';
import { RequestList } from '@/modules/spaces/components/request-list';

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
        {fixes.map((w) => (
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
            <RequestList path={`/spaces/${spaceId}/requests`} workspaceId={w.id} />
          </li>
        ))}
      </ul>
      {abandon.isError && <p className="text-sm text-danger">{abandon.error.message}</p>}
    </div>
  );
}
