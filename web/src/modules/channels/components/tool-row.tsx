import type { ReactNode } from 'react';
import type { Tool } from '@/modules/channels/utils/fold-events';
import { summarizeInput } from '@/modules/channels/utils/summarize-input';

function Mark({ tool }: { tool: Tool }) {
  if (!tool.done) {
    return (
      <svg
        className="mt-0.5 size-3 shrink-0 animate-spin text-working motion-reduce:animate-none"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="3"
        strokeLinecap="round"
        role="img"
        aria-label="running"
      >
        <path d="M12 3a9 9 0 1 1-9 9" />
      </svg>
    );
  }
  return tool.isError ? (
    <span className="text-failed" role="img" aria-label="error">
      ✕
    </span>
  ) : (
    <span className="text-done" role="img" aria-label="done">
      ✓
    </span>
  );
}

function Section({
  title,
  error,
  children,
}: {
  title: string;
  error?: boolean;
  children: ReactNode;
}) {
  return (
    <div>
      <div className={error ? 'text-failed' : 'text-muted'}>{title}</div>
      <pre className={`overflow-x-auto whitespace-pre-wrap ${error ? 'text-failed' : 'text-soft'}`}>
        {children}
      </pre>
    </div>
  );
}

export function ToolRow({ tool }: { tool: Tool }) {
  return (
    <div className="font-mono text-xs text-muted">
      <details>
        <summary className="flex cursor-pointer gap-2.5">
          <Mark tool={tool} />
          {tool.name}
          <span className="min-w-0 flex-1 truncate text-soft">{summarizeInput(tool.input)}</span>
          {tool.children.length > 0 && (
            <span>
              sub-agent · {tool.children.length} {tool.children.length === 1 ? 'tool' : 'tools'}
            </span>
          )}
        </summary>
        <div className="mt-1.5 space-y-2 rounded-lg bg-field px-2.5 py-2">
          <Section title="Input">{JSON.stringify(tool.input, null, 2)}</Section>
          {tool.done && (
            <Section title={tool.isError ? 'Error' : 'Result'} error={tool.isError}>
              {tool.output}
            </Section>
          )}
        </div>
      </details>
      {tool.children.length > 0 && (
        <div className="ml-1.5 mt-1.5 space-y-0.5 border-l border-line pl-3">
          {tool.children.map((c) => (
            <ToolRow key={c.id} tool={c} />
          ))}
        </div>
      )}
    </div>
  );
}
