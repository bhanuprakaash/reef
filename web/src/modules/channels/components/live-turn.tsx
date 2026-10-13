import { ToolRow } from '@/modules/channels/components/tool-row';
import type { LiveTurn as LiveTurnState } from '@/modules/channels/utils/fold-events';
import { errorClass, mutedClass } from '@/utils/classes';

export function LiveTurn({ turn, role }: { turn: LiveTurnState; role: string }) {
  if (turn.status === 'idle') return null;

  return (
    <div className="space-y-2 rounded border border-dashed border-neutral-300 p-3 dark:border-neutral-700">
      <div className="text-xs font-medium uppercase text-neutral-500">
        {role} {turn.status === 'running' && <span className="normal-case">is working...</span>}
      </div>
      {turn.items.map((item, i) =>
        item.type === 'text' ? (
          <p key={i} className="whitespace-pre-wrap text-sm">
            {item.text}
          </p>
        ) : (
          <ToolRow key={item.tool.id} tool={item.tool} />
        ),
      )}
      {turn.status === 'running' && turn.items.length === 0 && (
        <p className={mutedClass}>Waiting for the agent...</p>
      )}
      {turn.status === 'failed' && <p className={errorClass}>Failed: {turn.error}</p>}
    </div>
  );
}
