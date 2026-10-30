import { useInspector } from '@/modules/spaces/api/get-inspector';
import { formatTokens } from '@/modules/spaces/utils/format-tokens';
import { subagentLabel } from '@/modules/spaces/utils/subagent-label';
import type { InspectorToolCall, InspectorTurn, Usage } from '@/types/api';
import { Creature, faceOf, type Faces } from '@/components/creature';
import { EmptyState } from '@/components/empty-state';

const NO_COST = 'n/a';

function cost(costUsd: number | null) {
  return costUsd === null ? NO_COST : `$${costUsd.toFixed(4)}`;
}

function turnsLabel(n: number) {
  return `${n} turn${n === 1 ? '' : 's'}`;
}

function tokens(usage: Pick<Usage, 'inputTokens' | 'outputTokens'>) {
  return formatTokens(usage.inputTokens + usage.outputTokens);
}

function callLabel(call: InspectorToolCall) {
  let input: unknown;
  try {
    input = JSON.parse(call.input);
  } catch {
    return undefined;
  }
  return subagentLabel(call.name, input);
}

function ToolCallRow({ call }: { call: InspectorToolCall }) {
  const label = callLabel(call);
  const children = call.children ?? [];
  return (
    <div>
      <div className="text-soft">{label ? `${call.name} ${label}` : call.name}</div>
      {children.length > 0 && (
        <details className="mt-1.5">
          <summary className="cursor-pointer">
            sub-agent · {children.length} {children.length === 1 ? 'tool' : 'tools'}
          </summary>
          <div className="ml-1.5 mt-1.5 space-y-2 border-l border-line pl-3">
            {children.map((child, i) => (
              <ToolCallRow key={i} call={child} />
            ))}
          </div>
        </details>
      )}
      <pre className="overflow-x-auto whitespace-pre-wrap">{call.input}</pre>
      <pre className="overflow-x-auto whitespace-pre-wrap">{call.result}</pre>
    </div>
  );
}

function TurnRow({ turn, faces }: { turn: InspectorTurn; faces: Faces }) {
  const { usage } = turn;
  return (
    <details className="rounded-menu bg-field text-[12.5px]">
      <summary className="flex cursor-pointer items-center gap-2.5 px-3 py-2.5 text-[13.5px]">
        <Creature look={faceOf(faces, turn.botId)} size={20} />
        <span className="whitespace-nowrap">Turn {turn.id}</span>
        <span
          className={`min-w-0 truncate ${turn.status === 'failed' ? 'text-danger' : 'text-muted'}`}
        >
          {[turn.model, turn.status].filter(Boolean).join(' · ')}
        </span>
        <span className="ml-auto shrink-0 whitespace-nowrap tabular-nums text-muted">
          {tokens(usage)} · {cost(turn.costUsd)}
        </span>
      </summary>
      <div className="space-y-2 px-3 pb-3">
        <div className="flex flex-wrap gap-x-3 tabular-nums text-muted">
          <span>in {formatTokens(usage.inputTokens)}</span>
          <span>out {formatTokens(usage.outputTokens)}</span>
          <span>cache read {formatTokens(usage.cacheReadTokens)}</span>
          <span>cache write {formatTokens(usage.cacheWriteTokens)}</span>
        </div>
        <details className="rounded-field bg-main">
          <summary className="cursor-pointer px-2.5 py-2 text-soft">Prompt</summary>
          <pre className="overflow-x-auto whitespace-pre-wrap px-2.5 pb-2.5 font-mono text-muted">
            {turn.prompt}
          </pre>
        </details>
        {turn.toolCalls.length > 0 && (
          <div className="space-y-2 rounded-field bg-main px-2.5 py-2 font-mono text-muted">
            {turn.toolCalls.map((call, i) => (
              <ToolCallRow key={i} call={call} />
            ))}
          </div>
        )}
      </div>
    </details>
  );
}

export function InspectorPanel({
  spaceId,
  faces,
  onClose,
}: {
  spaceId: number;
  faces: Faces;
  onClose: () => void;
}) {
  const inspector = useInspector(spaceId, true);
  const data = inspector.data;
  const roles = data?.roles ?? [];
  const costs = roles.flatMap((r) => (r.costUsd === null ? [] : [r.costUsd]));
  const total = cost(costs.length === 0 ? null : costs.reduce((a, b) => a + b, 0));
  const costTotal =
    costs.length > 0 && costs.length < roles.length ? `${total} + ${NO_COST}` : total;
  const totals = [
    ['Cost', costTotal],
    [
      'Tokens',
      formatTokens(roles.reduce((n, r) => n + r.usage.inputTokens + r.usage.outputTokens, 0)),
    ],
    ['Turns', String(roles.reduce((n, r) => n + r.turns, 0))],
  ];

  return (
    <aside aria-label="Inspector" className="space-y-[18px]">
      <div className="flex items-center gap-2">
        <h2 className="text-[15px] font-normal">Inspector</h2>
        <span className="text-[12.5px] text-muted">this Space</span>
        <button
          type="button"
          aria-label="Close inspector"
          onClick={onClose}
          className="ml-auto flex size-9 items-center justify-center rounded-field text-soft hover:bg-card"
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
            <path d="M6 6l12 12M18 6L6 18" />
          </svg>
        </button>
      </div>
      {inspector.isPending && <p className="text-sm text-muted">Loading...</p>}
      {inspector.isError && <p className="text-sm text-danger">{inspector.error.message}</p>}
      {data && roles.length === 0 && (
        <EmptyState
          title="No turns yet"
          body="Tokens and tools show up after the first turn."
          face="shark"
          compact
        />
      )}
      {data && roles.length > 0 && (
        <>
          <div className="grid grid-cols-3 gap-2">
            {totals.map(([label, value]) => (
              <div key={label} className="min-w-0 rounded-menu bg-field p-2.5">
                <div className="text-xs text-muted">{label}</div>
                <div className="text-[17px] tabular-nums">{value}</div>
              </div>
            ))}
          </div>
          <div>
            <h3 className="mb-1.5 text-[12.5px] font-normal text-muted">By role</h3>
            <ul className="text-[13.5px] tabular-nums">
              {roles.map((role) => (
                <li key={role.botId ?? role.role} className="flex min-h-9 items-center gap-2">
                  <Creature look={faceOf(faces, role.botId)} size={20} />
                  <span className="min-w-0 flex-1 truncate capitalize">{role.role}</span>
                  <span className="text-right text-muted">{turnsLabel(role.turns)}</span>
                  <span className="w-11 text-right text-muted">{tokens(role.usage)}</span>
                  <span
                    className={`w-[4.5rem] text-right ${role.costUsd === null ? 'text-muted' : ''}`}
                  >
                    {cost(role.costUsd)}
                  </span>
                </li>
              ))}
            </ul>
            {roles.some((r) => r.costUsd === null) && (
              <p className="mt-1.5 text-xs text-faint">{NO_COST}: Codex does not report cost</p>
            )}
          </div>
          <div>
            <h3 className="mb-1.5 text-[12.5px] font-normal text-muted">Turns</h3>
            <div className="space-y-1.5">
              {[...data.turns]
                .sort((a, b) => b.id - a.id)
                .map((turn) => (
                  <TurnRow key={turn.id} turn={turn} faces={faces} />
                ))}
            </div>
          </div>
        </>
      )}
    </aside>
  );
}
