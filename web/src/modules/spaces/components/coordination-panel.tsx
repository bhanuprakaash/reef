import { useContinueSpace } from '@/lib/api/continue-space';
import { useRetryTurn } from '@/lib/api/retry-turn';
import { Bubble } from '@/modules/spaces/components/bubble';
import { ToolRow } from '@/modules/spaces/components/tool-row';
import { dotFor, type Chain } from '@/modules/spaces/utils/direct-chains';
import type { LiveTurn } from '@/modules/spaces/utils/fold-events';
import { Button } from '@/components/button';
import { Creature, faceOf, type Faces } from '@/components/creature';
import { StatusDot } from '@/components/status-dot';

type Props = {
  readOnly?: boolean;
  spaceId: number;
  chain: Chain;
  live?: LiveTurn;
  faces: Faces;
  running: boolean;
  paused: boolean;
  reason: string;
  onOpenSource: (messageId: number) => void;
  onClose: () => void;
};

export function CoordinationPanel({
  readOnly = false,
  spaceId,
  chain,
  live,
  faces,
  running,
  paused,
  reason,
  onOpenSource,
  onClose,
}: Props) {
  const cont = useContinueSpace();
  const retry = useRetryTurn();
  const { retryTurn } = chain;
  const { from, to, sourceMessageId } = chain.messages[0];
  const error = [cont, retry].find((m) => m.isError)?.error;

  return (
    <aside
      aria-label="Coordination"
      className="flex min-h-0 flex-1 flex-col bg-sidebar max-md:bg-main"
    >
      <div className="flex min-h-14 items-center gap-2 border-b border-hairline py-2 pl-[18px] pr-3 max-md:pl-2">
        <h2 className="text-[15px] font-normal">Coordination</h2>
        <span className="flex items-center gap-1.5 text-[12.5px] text-muted">
          <StatusDot status={dotFor[chain.state]} />
          {chain.state}
        </span>
        <Button
          variant="ghost"
          size="icon"
          type="button"
          aria-label="Close coordination"
          className="ml-auto max-md:order-first max-md:ml-0"
          onClick={onClose}
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path className="max-md:hidden" d="M6 6l12 12M18 6L6 18" />
            <path className="md:hidden" d="M15 5l-7 7 7 7" />
          </svg>
        </Button>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-[18px] py-4">
        <p className="text-[13px] text-muted">
          <span className="capitalize">{from}</span> and <span className="capitalize">{to}</span>{' '}
          exchanged these messages privately.
          {sourceMessageId !== null && (
            <>
              {' '}
              <button
                type="button"
                className="border-b border-line text-fg"
                onClick={() => onOpenSource(sourceMessageId)}
              >
                Open the message that started it
              </button>
            </>
          )}
        </p>
        <ol className="flex flex-col gap-3">
          {chain.messages.map((m) => (
            <li
              key={m.id}
              className="rounded-card border border-line bg-card px-4 py-3 [--creature-ring:var(--color-card)]"
            >
              <div className="flex items-center gap-2 text-[13px] text-muted">
                <Creature look={faceOf(faces, m.fromBotId)} size={20} />
                <span className="capitalize">{`${m.from} → ${m.to}`}</span>
                <span className="ml-auto">
                  {new Date(m.createdAt).toLocaleTimeString([], {
                    hour: '2-digit',
                    minute: '2-digit',
                  })}
                  {` · ${m.status}`}
                </span>
              </div>
              <p className="mt-2 whitespace-pre-wrap text-[14.5px]">{m.body}</p>
            </li>
          ))}
        </ol>
        {live && (
          <div className="flex flex-col items-start gap-2.5">
            {live.items.map((item, i) =>
              item.type === 'text' ? (
                <Bubble key={i}>{item.text}</Bubble>
              ) : (
                <div key={item.tool.id} className="w-full">
                  <ToolRow tool={item.tool} />
                </div>
              ),
            )}
          </div>
        )}
        {chain.state === 'paused' && (
          <div className="flex flex-wrap items-center gap-3">
            <p className="min-w-0 flex-1 basis-48 text-[13px] text-paused-fg">Paused: {reason}</p>
            {!readOnly && (
              <Button
                variant="paused"
                type="button"
                disabled={cont.isPending}
                onClick={() => cont.mutate(spaceId)}
              >
                Continue
              </Button>
            )}
          </div>
        )}
        {retryTurn && !paused && (chain.state === 'failed' || chain.state === 'stopped') && (
          <div>
            <Button
              variant="primary"
              type="button"
              disabled={running || retry.isPending}
              onClick={() => retry.mutate(retryTurn.id)}
            >
              Retry
            </Button>
          </div>
        )}
        {error && <p className="text-sm text-danger">{error.message}</p>}
      </div>
    </aside>
  );
}
