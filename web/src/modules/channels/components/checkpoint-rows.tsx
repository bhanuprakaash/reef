import { useTurns } from '@/modules/channels/api/get-turns';
import { useRollbackTurn } from '@/modules/channels/api/rollback-turn';
import { Button } from '@/components/button';

export function CheckpointRows({ channelId, running }: { channelId: number; running: boolean }) {
  const turns = useTurns(channelId);
  const rollback = useRollbackTurn(channelId);
  const checkpoints = [...(turns.data ?? [])].reverse();

  if (checkpoints.length === 0) return null;

  return (
    <section aria-label="Checkpoints" className="space-y-2 text-sm">
      <h2 className="font-medium">Checkpoints</h2>
      <ul className="max-h-48 space-y-2 overflow-y-auto">
        {checkpoints.map((t) => (
          <li
            key={t.id}
            className="flex items-center justify-between gap-3 rounded border border-line p-3"
          >
            <p>
              Before turn {t.id} by {t.role}
            </p>
            <Button
              variant="destructive"
              type="button"
              disabled={running || rollback.isPending}
              onClick={() => {
                if (
                  window.confirm(
                    `Roll back to before turn ${t.id}? The files and the branch go back to that point, and later commits are dropped. The agent's session is not rewound.`,
                  )
                ) {
                  rollback.mutate(t.id);
                }
              }}
            >
              Rollback
            </Button>
          </li>
        ))}
      </ul>
      {rollback.isError && <p className="text-sm text-danger">{rollback.error.message}</p>}
    </section>
  );
}
