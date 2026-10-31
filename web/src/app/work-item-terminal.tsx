import { TerminalPane } from '@/app/terminal-pane';
import { useEnsureWorkspaces } from '@/modules/work-items/api/ensure-workspaces';
import { useWorkspaces } from '@/modules/work-items/api/get-workspaces';
import { Button } from '@/components/button';

// WorkItemTerminal opens the shell in the Work item's first ready workspace when
// there is one. Without any it asks first, so showing the terminal never makes a workspace by
// itself. Key it by Work item.
export function WorkItemTerminal({
  workItemId,
  onHide,
}: {
  workItemId: number;
  onHide: () => void;
}) {
  const workspaces = useWorkspaces(workItemId);
  const ensure = useEnsureWorkspaces(workItemId);
  const first = workspaces.data?.find((w) => w.state === 'ready');
  if (first) return <TerminalPane cwd={first.dir} onHide={onHide} />;
  if (!workspaces.data && !workspaces.isError) return null;
  const error = workspaces.error ?? ensure.error;
  return (
    <section aria-label="Terminal" className="bg-terminal">
      <div className="flex items-center gap-3 px-4 py-3 text-[12.5px] text-muted">
        <span>{error ? error.message : 'No workspace yet'}</span>
        <Button
          type="button"
          size="sm"
          className="ml-auto"
          loading={ensure.isPending}
          onClick={() => ensure.mutate()}
        >
          Open terminal
        </Button>
      </div>
    </section>
  );
}
