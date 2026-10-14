import { useFailedTurns } from '@/modules/channels/api/get-failed-turns';
import { useRetryTurn } from '@/lib/api/retry-turn';
import { failedTurnAnchor } from '@/utils/anchors';
import { buttonClass, errorClass } from '@/utils/classes';

export function FailedTurnRows({
  channelId,
  running,
  paused,
}: {
  channelId: number;
  running: boolean;
  paused: boolean;
}) {
  const turns = useFailedTurns(channelId);
  const retry = useRetryTurn();
  const retryable = turns.data ?? [];

  if (retryable.length === 0) return null;

  return (
    <div className="space-y-2">
      <ul className="space-y-2">
        {retryable.map((t) => (
          <li
            key={t.id}
            id={failedTurnAnchor(t.id)}
            className="flex items-center justify-between gap-3 rounded border border-red-300 p-3 text-sm dark:border-red-800"
          >
            <p className={errorClass}>
              Turn {t.id} by {t.role} failed: {t.error}
            </p>
            {!paused && (
              <button
                type="button"
                className={buttonClass}
                disabled={running || retry.isPending}
                onClick={() => retry.mutate(t.id)}
              >
                Retry
              </button>
            )}
          </li>
        ))}
      </ul>
      {retry.isError && <p className={errorClass}>{retry.error.message}</p>}
    </div>
  );
}
