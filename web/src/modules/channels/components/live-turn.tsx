import { Creature } from '@/components/creature';
import { Bubble } from '@/modules/channels/components/bubble';
import { ToolRow } from '@/modules/channels/components/tool-row';
import type { LiveTurn as LiveTurnState } from '@/modules/channels/utils/fold-events';

type Props = { turn: LiveTurnState; role: string; face?: string };

export function LiveTurn({ turn, role, face }: Props) {
  if (turn.status === 'idle') return null;

  return (
    <div className="flex flex-col items-start gap-2.5">
      <div className="mt-2.5 flex items-center gap-2 text-[13px] capitalize text-muted">
        <Creature
          role={role}
          face={face}
          size={20}
          state={turn.status === 'running' ? 'working' : 'failed'}
        />
        <span>
          {role} {turn.status === 'running' && <span className="normal-case">is working...</span>}
        </span>
      </div>
      {turn.items.map((item, i) =>
        item.type === 'text' ? (
          turn.status !== 'failed' && <Bubble key={i}>{item.text}</Bubble>
        ) : (
          <div key={item.tool.id} className="w-full pl-7">
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
      {turn.status === 'failed' && <p className="text-sm text-danger">Failed: {turn.error}</p>}
    </div>
  );
}
