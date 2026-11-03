import { Button } from '@/components/button';
import { StatusDot } from '@/components/status-dot';
import { useRepoConnections } from '@/lib/api/get-repo-connections';
import {
  useCheckRepoConnection,
  useConnectRepo,
} from '@/modules/repo-connections/api/connect-repo';
import type { RepoProvider } from '@/types/api';

const providers: { provider: RepoProvider; label: string; host: string }[] = [
  { provider: 'github', label: 'GitHub', host: 'github.com' },
  { provider: 'gitlab', label: 'GitLab', host: 'gitlab.com' },
];

export function RepoConnectionList() {
  const connections = useRepoConnections();
  const connect = useConnectRepo();
  const check = useCheckRepoConnection();
  const error = connections.error ?? connect.error ?? check.error;

  return (
    <>
      {connections.isPending && <p className="text-sm text-muted">Loading...</p>}
      <ul className="overflow-hidden rounded-card border border-hairline bg-sidebar">
        {providers.map((p) => {
          const c = connections.data?.find((x) => x.provider === p.provider && x.host === p.host);
          const pending = connect.variables === p.provider && connect.isPending;
          const checking = c !== undefined && check.variables === c.id && check.isPending;
          return (
            <li
              key={p.provider}
              className="flex items-center gap-3 border-t border-hairline px-3.5 py-3 first:border-t-0"
            >
              <span className="min-w-0 flex-1">
                <span className="block text-[13px] font-medium">
                  {p.label} <span className="font-normal text-muted">{p.host}</span>
                </span>
                <span
                  className={`block text-xs ${c?.state === 'disconnected' ? 'text-danger' : 'text-muted'}`}
                >
                  {c === undefined && 'Not connected'}
                  {c?.state === 'connected' && `Connected as ${c.account}`}
                  {c?.state === 'disconnected' && `Disconnected: ${c.reason}`}
                </span>
              </span>
              {c && (
                <StatusDot
                  status={c.state === 'connected' ? 'done' : 'failed'}
                  label={c.state === 'connected' ? 'Connected' : 'Disconnected'}
                />
              )}
              <Button
                size="sm"
                loading={pending || checking}
                onClick={() => (c ? check.mutate(c.id) : connect.mutate(p.provider))}
              >
                {c ? 'Re-check' : 'Connect'}
              </Button>
            </li>
          );
        })}
      </ul>
      {error && <p className="text-sm text-danger">{error.message}</p>}
    </>
  );
}
