import { useContinueChannel } from '@/lib/api/continue-channel';
import { pausedAnchor } from '@/utils/anchors';
import { Button } from '@/components/button';

export function PausedBanner({ channelId, reason }: { channelId: number; reason: string }) {
  const cont = useContinueChannel();

  return (
    <div
      id={pausedAnchor}
      role="alert"
      className="flex flex-wrap items-center gap-3 rounded-card border border-paused-line bg-paused-bg px-4 py-3.5"
    >
      <svg
        width="20"
        height="20"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        aria-hidden="true"
        className="text-paused"
      >
        <rect x="6" y="5" width="4" height="14" rx="1" />
        <rect x="14" y="5" width="4" height="14" rx="1" />
      </svg>
      <div className="min-w-0 flex-1 basis-48">
        <p className="text-paused-fg">Paused: {reason}</p>
        {cont.isError && <p className="text-sm text-danger">{cont.error.message}</p>}
      </div>
      <Button
        variant="paused"
        type="button"
        disabled={cont.isPending}
        onClick={() => cont.mutate(channelId)}
      >
        Continue
      </Button>
    </div>
  );
}
