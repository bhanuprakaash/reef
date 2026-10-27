import { TerminalPane } from '@/app/terminal-pane';
import { useEnsureWorktree } from '@/modules/work-items/api/ensure-worktree';
import { useWorktree } from '@/modules/work-items/api/get-worktree';
import { Button } from '@/components/button';

// WorkItemTerminal opens the shell in the feature's worktree when there is one. Without one it
// asks first, so showing the terminal never makes a worktree by itself. Key it by feature.
export function WorkItemTerminal({
  workItemId,
  onHide,
}: {
  workItemId: number;
  onHide: () => void;
}) {
  const worktree = useWorktree(workItemId);
  const ensure = useEnsureWorktree(workItemId);
  if (worktree.data?.path) return <TerminalPane cwd={worktree.data.path} onHide={onHide} />;
  if (!worktree.data && !worktree.isError) return null;
  const error = worktree.error ?? ensure.error;
  return (
    <section aria-label="Terminal" className="bg-terminal">
      <div className="flex items-center gap-3 px-4 py-3 text-[12.5px] text-muted">
        <span>{error ? error.message : 'No worktree yet'}</span>
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
