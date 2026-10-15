import { Bubble } from '@/modules/channels/components/bubble';
import { ToolRow } from '@/modules/channels/components/tool-row';
import { useStickToBottom } from '@/modules/channels/hooks/use-stick-to-bottom';
import type { LiveTurn } from '@/modules/channels/utils/fold-events';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';

type Props = { turn: LiveTurn; role: string; face?: string; onClose: () => void };

export function TurnPanel({ turn, role, face, onClose }: Props) {
  const { box, onScroll } = useStickToBottom();
  const failed = turn.status === 'failed';

  return (
    <aside aria-label="Turn" className="flex min-h-0 flex-1 flex-col bg-sidebar max-md:bg-main">
      <div className="flex min-h-14 items-center gap-2.5 border-b border-hairline py-2 pl-[18px] pr-3 max-md:pl-2">
        <Creature
          role={role}
          face={face}
          size={22}
          state={turn.status === 'running' ? 'working' : failed ? 'failed' : 'idle'}
        />
        <h2 className="text-[15px] font-normal capitalize">{role}</h2>
        <span className="text-[12.5px] text-muted">
          {turn.status === 'running' ? 'working now' : failed ? 'failed' : 'finished'}
        </span>
        <Button
          variant="ghost"
          size="icon"
          type="button"
          aria-label="Close turn"
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
      <div ref={box} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto">
        <div className="flex flex-col items-start gap-2.5 px-[18px] py-4">
          {turn.items.map((item, i) =>
            item.type === 'text' ? (
              !failed && <Bubble key={i}>{item.text}</Bubble>
            ) : (
              <div key={item.tool.id} className="w-full">
                <ToolRow tool={item.tool} />
              </div>
            ),
          )}
          {turn.status === 'running' && turn.items.length === 0 && (
            <div
              role="status"
              aria-label="Waiting for the agent"
              className="flex gap-1.5 rounded-[22px] bg-bubble px-[18px] py-3.5"
            >
              {['0s', '0.2s', '0.4s'].map((delay) => (
                <span
                  key={delay}
                  style={{ animationDelay: delay }}
                  className="size-[7px] animate-pulse rounded-full bg-soft motion-reduce:animate-none"
                />
              ))}
            </div>
          )}
          {failed && <p className="text-sm text-danger">Failed: {turn.error}</p>}
        </div>
      </div>
    </aside>
  );
}
