import { Creature } from '@/components/creature';
import { ToolRow } from '@/modules/channels/components/tool-row';
import type { LiveTurn as LiveTurnState } from '@/modules/channels/utils/fold-events';

export function LiveTurn({ turn, role }: { turn: LiveTurnState; role: string }) {
  if (turn.status === 'idle') return null;

  return (
    <div className="space-y-2 rounded border border-dashed border-line p-3">
      <div className="flex items-center gap-1.5 text-xs font-medium uppercase text-muted">
        <Creature role={role} size={20} state={turn.status === 'running' ? 'working' : 'failed'} />
        {role} {turn.status === 'running' && <span className="normal-case">is working...</span>}
      </div>
      {turn.items.map((item, i) =>
        item.type === 'text' ? (
          turn.status !== 'failed' && (
            <p key={i} className="whitespace-pre-wrap text-sm">
              {item.text}
            </p>
          )
        ) : (
          <ToolRow key={item.tool.id} tool={item.tool} />
        ),
      )}
      {turn.status === 'running' && turn.items.length === 0 && (
        <p className="text-sm text-muted">Waiting for the agent...</p>
      )}
      {turn.status === 'failed' && <p className="text-sm text-danger">Failed: {turn.error}</p>}
    </div>
  );
}
