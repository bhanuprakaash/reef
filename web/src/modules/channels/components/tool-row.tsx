import type { Tool } from '@/modules/channels/utils/fold-events';
import { summarizeInput } from '@/modules/channels/utils/summarize-input';

export function ToolRow({ tool }: { tool: Tool }) {
  const state = tool.done ? (tool.isError ? 'error' : 'done') : 'running';
  return (
    <div>
      <details className="rounded border border-hairline text-sm">
        <summary className="flex cursor-pointer gap-2 px-2 py-1">
          <span className="font-mono font-medium">{tool.name}</span>
          <span className="min-w-0 flex-1 truncate font-mono text-muted">
            {summarizeInput(tool.input)}
          </span>
          <span className={tool.isError ? 'text-sm text-danger' : 'text-sm text-muted'}>
            {state}
          </span>
        </summary>
        <div className="space-y-2 border-t border-hairline p-2">
          <div>
            <div className="text-sm text-muted">Input</div>
            <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-xs">
              {JSON.stringify(tool.input, null, 2)}
            </pre>
          </div>
          {tool.done && (
            <div>
              <div className={tool.isError ? 'text-sm text-danger' : 'text-sm text-muted'}>
                {tool.isError ? 'Error' : 'Result'}
              </div>
              <pre
                className={`overflow-x-auto whitespace-pre-wrap font-mono text-xs ${tool.isError ? 'text-danger' : ''}`}
              >
                {tool.output}
              </pre>
            </div>
          )}
        </div>
      </details>
      {tool.children.length > 0 && (
        <div className="ml-4 mt-1 space-y-1 border-l border-hairline pl-2">
          {tool.children.map((c) => (
            <ToolRow key={c.id} tool={c} />
          ))}
        </div>
      )}
    </div>
  );
}
