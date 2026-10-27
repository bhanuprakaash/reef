import { useFailedTurns } from '@/modules/spaces/api/get-failed-turns';
import { useTurns } from '@/modules/spaces/api/get-turns';
import { useRollbackTurn } from '@/modules/spaces/api/rollback-turn';
import { useRetryTurn } from '@/lib/api/retry-turn';
import { confirmRollback } from '@/modules/spaces/utils/confirm-rollback';
import { failedTurnAnchor } from '@/utils/anchors';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';

export function FailedTurnRows({
  spaceId,
  faces,
  running,
  paused,
}: {
  spaceId: number;
  faces: Record<string, string | undefined>;
  running: boolean;
  paused: boolean;
}) {
  const turns = useFailedTurns(spaceId);
  const checkpoints = useTurns(spaceId);
  const retry = useRetryTurn();
  const rollback = useRollbackTurn(spaceId);
  const retryable = turns.data ?? [];
  const rollbackable = new Set((checkpoints.data ?? []).map((t) => t.id));

  if (retryable.length === 0) return null;

  return (
    <div className="space-y-2">
      <ul className="space-y-2">
        {retryable.map((t) => (
          <li
            key={t.id}
            id={failedTurnAnchor(t.id)}
            className="flex flex-wrap items-center gap-3 rounded-card border border-failed-line bg-failed-bg px-4 py-3.5 [--creature-ring:var(--color-failed-bg)]"
          >
            <Creature role={t.role} face={faces[t.role]} state="failed" size={24} />
            <div className="min-w-0 flex-1 basis-48">
              <p className="text-failed-fg">
                <span className="capitalize">{t.role}</span>
                {`'s turn ${t.id} failed`}
              </p>
              <p className="text-[13px] text-failed-soft">{t.error}</p>
            </div>
            {rollbackable.has(t.id) && (
              <Button
                variant="failed"
                type="button"
                disabled={running || rollback.isPending}
                onClick={() => confirmRollback(t.id) && rollback.mutate(t.id)}
              >
                Roll back
              </Button>
            )}
            {!paused && (
              <Button
                variant="primary"
                type="button"
                disabled={running || retry.isPending}
                onClick={() => retry.mutate(t.id)}
              >
                Retry
              </Button>
            )}
          </li>
        ))}
      </ul>
      {retry.isError && <p className="text-sm text-danger">{retry.error.message}</p>}
      {rollback.isError && <p className="text-sm text-danger">{rollback.error.message}</p>}
    </div>
  );
}
