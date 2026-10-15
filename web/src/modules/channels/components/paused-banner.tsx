import { useContinueChannel } from '@/lib/api/continue-channel';
import { pausedAnchor } from '@/utils/anchors';
import { Button } from '@/components/button';

export function PausedBanner({ channelId, reason }: { channelId: number; reason: string }) {
  const cont = useContinueChannel();

  return (
    <div
      id={pausedAnchor}
      role="alert"
      className="flex items-center justify-between gap-3 rounded border border-paused bg-paused/10 p-3 text-sm"
    >
      <div>
        <p className="font-medium">Paused: {reason}</p>
        {cont.isError && <p className="text-sm text-danger">{cont.error.message}</p>}
      </div>
      <Button type="button" disabled={cont.isPending} onClick={() => cont.mutate(channelId)}>
        Continue
      </Button>
    </div>
  );
}
