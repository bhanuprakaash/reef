import { useContinueChannel } from '@/modules/channels/api/continue-channel';
import { buttonClass, errorClass } from '@/utils/classes';

export function PausedBanner({ channelId, reason }: { channelId: number; reason: string }) {
  const cont = useContinueChannel(channelId);

  return (
    <div
      role="alert"
      className="flex items-center justify-between gap-3 rounded border border-amber-400 bg-amber-50 p-3 text-sm dark:border-amber-600 dark:bg-amber-950"
    >
      <div>
        <p className="font-medium">Paused: {reason}</p>
        {cont.isError && <p className={errorClass}>{cont.error.message}</p>}
      </div>
      <button
        type="button"
        className={buttonClass}
        disabled={cont.isPending}
        onClick={() => cont.mutate()}
      >
        Continue
      </button>
    </div>
  );
}
