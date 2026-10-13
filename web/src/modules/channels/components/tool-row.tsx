import type { Tool } from '@/modules/channels/utils/fold-events';
import { summarizeInput } from '@/modules/channels/utils/summarize-input';
import { errorClass, mutedClass } from '@/utils/classes';

export function ToolRow({ tool }: { tool: Tool }) {
  const state = tool.done ? (tool.isError ? 'error' : 'done') : 'running';
  return (
    <div>
      <details className="rounded border border-neutral-200 text-sm dark:border-neutral-800">
        <summary className="flex cursor-pointer gap-2 px-2 py-1">
          <span className="font-mono font-medium">{tool.name}</span>
          <span className="min-w-0 flex-1 truncate font-mono text-neutral-500">
            {summarizeInput(tool.input)}
          </span>
          <span className={tool.isError ? errorClass : mutedClass}>{state}</span>
        </summary>
        <div className="space-y-2 border-t border-neutral-200 p-2 dark:border-neutral-800">
          <div>
            <div className={mutedClass}>Input</div>
            <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-xs">
              {JSON.stringify(tool.input, null, 2)}
            </pre>
          </div>
          {tool.done && (
            <div>
              <div className={tool.isError ? errorClass : mutedClass}>
                {tool.isError ? 'Error' : 'Result'}
              </div>
              <pre
                className={`overflow-x-auto whitespace-pre-wrap font-mono text-xs ${tool.isError ? errorClass : ''}`}
              >
                {tool.output}
              </pre>
            </div>
          )}
        </div>
      </details>
      {tool.children.length > 0 && (
        <div className="ml-4 mt-1 space-y-1 border-l border-neutral-200 pl-2 dark:border-neutral-800">
          {tool.children.map((c) => (
            <ToolRow key={c.id} tool={c} />
          ))}
        </div>
      )}
    </div>
  );
}
