import type { Turn } from '@/types/api';
import { useRollbackTurn } from '@/modules/spaces/api/rollback-turn';
import { checkpointItemLabel } from '@/modules/spaces/utils/checkpoint-label';
import { confirmRollback } from '@/modules/spaces/utils/confirm-rollback';

const icon = {
  width: 15,
  height: 15,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.8,
  strokeLinecap: 'round',
  'aria-hidden': true,
} as const;

export function CheckpointButton({
  spaceId,
  turn,
  running,
}: {
  spaceId: number;
  turn: Turn;
  running: boolean;
}) {
  const rollback = useRollbackTurn(spaceId);
  const label = checkpointItemLabel(turn);
  return (
    <>
      <button
        type="button"
        aria-label={`Roll back · ${label}`}
        title={label}
        disabled={running || rollback.isPending}
        className="flex size-7 items-center justify-center rounded-field text-soft hover:bg-card disabled:cursor-not-allowed disabled:text-faint"
        onClick={() => confirmRollback(turn.id) && rollback.mutate(turn.id)}
      >
        <svg {...icon}>
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2v6M12 16v6" />
        </svg>
      </button>
      {rollback.isError && (
        <span className="text-[13px] text-danger">{rollback.error.message}</span>
      )}
    </>
  );
}
