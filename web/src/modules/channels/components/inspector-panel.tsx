import { useInspector } from '@/modules/channels/api/get-inspector';
import type { InspectorRole, InspectorTurn } from '@/types/api';
import { buttonClass, errorClass, mutedClass } from '@/utils/classes';

function cost(costUsd: number | null) {
  return costUsd === null ? '—' : `$${costUsd.toFixed(2)}`;
}

function tokens(usage: InspectorRole['usage']) {
  return `${usage.inputTokens} in / ${usage.outputTokens} out`;
}

function TurnRow({ turn }: { turn: InspectorTurn }) {
  return (
    <details className="rounded border border-neutral-200 text-sm dark:border-neutral-800">
      <summary className="flex cursor-pointer gap-2 px-2 py-1">
        <span className="min-w-0 flex-1 truncate">{turn.model || turn.status}</span>
        <span className={mutedClass}>
          {tokens(turn.usage)} · {cost(turn.costUsd)}
        </span>
      </summary>
      <div className="space-y-2 border-t border-neutral-200 p-2 dark:border-neutral-800">
        <div>
          <div className={mutedClass}>Prompt</div>
          <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-xs">{turn.prompt}</pre>
        </div>
        {turn.toolCalls.map((call, i) => (
          <div key={i}>
            <div className="font-mono text-xs font-medium">{call.name}</div>
            <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-xs">
              {call.input}
            </pre>
            <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-xs">
              {call.result}
            </pre>
          </div>
        ))}
      </div>
    </details>
  );
}

function RoleBlock({ role, turns }: { role: InspectorRole; turns: InspectorTurn[] }) {
  const sorted = [...turns].sort((a, b) => b.id - a.id);
  return (
    <details className="rounded border border-neutral-200 text-sm dark:border-neutral-800">
      <summary className="flex cursor-pointer gap-2 px-2 py-1">
        <span className="font-medium">{role.role}</span>
        <span className={mutedClass}>
          {role.turns} turns · {tokens(role.usage)} · {cost(role.costUsd)}
        </span>
      </summary>
      <div className="space-y-2 border-t border-neutral-200 p-2 dark:border-neutral-800">
        {sorted.map((turn) => (
          <TurnRow key={turn.id} turn={turn} />
        ))}
      </div>
    </details>
  );
}

export function InspectorPanel({ channelId, onClose }: { channelId: number; onClose: () => void }) {
  const inspector = useInspector(channelId, true);

  return (
    <aside
      aria-label="Inspector"
      className="w-80 shrink-0 space-y-3 self-start rounded border border-neutral-300 p-3 dark:border-neutral-700"
    >
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium">Inspector</h2>
        <button type="button" className={buttonClass} onClick={onClose}>
          Close
        </button>
      </div>
      {inspector.isPending && <p className={mutedClass}>Loading...</p>}
      {inspector.isError && <p className={errorClass}>{inspector.error.message}</p>}
      {inspector.data && inspector.data.roles.length === 0 && (
        <p className={mutedClass}>No turns yet.</p>
      )}
      {inspector.data && (
        <div className="space-y-2">
          {inspector.data.roles.map((role) => (
            <RoleBlock
              key={role.role}
              role={role}
              turns={inspector.data.turns.filter((turn) => turn.role === role.role)}
            />
          ))}
        </div>
      )}
    </aside>
  );
}
