import { useAvailableConnections, useImportConnection } from '@/modules/teams/api/connections';
import { availableLabel } from '@/modules/teams/utils/connections';
import type { AvailableConnection } from '@/types/api';
import { Button } from '@/components/button';

export function AvailableRow({
  server,
  importing,
  onAdd,
}: {
  server: AvailableConnection;
  importing: boolean;
  onAdd: () => void;
}) {
  return (
    <li className="flex items-center gap-3 py-3">
      <span className="min-w-0 flex-1">
        <span className="block font-mono text-[13px]">{server.name}</span>
        <span className="block truncate text-xs text-muted">
          {server.source} · {server.kind === 'url' ? 'HTTP' : 'Local process'} ·{' '}
          {availableLabel(server)}
        </span>
      </span>
      {server.added ? (
        <span className="shrink-0 text-xs text-muted">Added</span>
      ) : (
        <Button
          size="sm"
          aria-label={`Add ${server.name} from ${server.source}`}
          loading={importing}
          onClick={onAdd}
        >
          Add
        </Button>
      )}
    </li>
  );
}

export function AvailableConnections({ projectId }: { projectId?: number }) {
  const available = useAvailableConnections(projectId);
  const add = useImportConnection();
  return (
    <div className="flex flex-col gap-1">
      <h3 className="text-sm font-medium">Available on this Mac</h3>
      {available.isError && <p className="text-sm text-danger">{available.error.message}</p>}
      {available.data?.length === 0 && (
        <p className="text-sm text-muted">No MCP servers found on this Mac.</p>
      )}
      <ul className="flex flex-col divide-y divide-hairline">
        {available.data?.map((s) => (
          <AvailableRow
            key={`${s.source}/${s.name}`}
            server={s}
            importing={
              add.isPending && add.variables.source === s.source && add.variables.name === s.name
            }
            onAdd={() => add.mutate({ source: s.source, name: s.name, projectId })}
          />
        ))}
      </ul>
      {add.isError && <p className="text-sm text-danger">{add.error.message}</p>}
    </div>
  );
}
