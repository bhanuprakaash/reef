import { useTurns } from '@/modules/channels/api/get-turns';
import { useRollbackTurn } from '@/modules/channels/api/rollback-turn';
import { confirmRollback } from '@/modules/channels/utils/confirm-rollback';

export function CheckpointRows({ channelId, running }: { channelId: number; running: boolean }) {
  const turns = useTurns(channelId);
  const rollback = useRollbackTurn(channelId);
  const checkpoints = turns.data ?? [];

  if (checkpoints.length === 0) return null;

  return (
    <section aria-label="Checkpoints" className="space-y-2 text-sm">
      <ul className="max-h-48 space-y-3 overflow-y-auto">
        {checkpoints.map((t) => (
          <li key={t.id} className="flex items-center gap-2.5 text-[13px] text-muted">
            <span className="h-px flex-1 bg-track" />
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <circle cx="12" cy="12" r="4" />
              <path d="M12 2v6M12 16v6" />
            </svg>
            <span>
              Checkpoint before turn {t.id} by {t.role} ·{' '}
              <button
                type="button"
                className="border-b border-line text-fg disabled:cursor-not-allowed disabled:text-faint"
                disabled={running || rollback.isPending}
                onClick={() => confirmRollback(t.id) && rollback.mutate(t.id)}
              >
                Roll back
              </button>
            </span>
            <span className="h-px flex-1 bg-track" />
          </li>
        ))}
      </ul>
      {rollback.isError && <p className="text-sm text-danger">{rollback.error.message}</p>}
    </section>
  );
}
