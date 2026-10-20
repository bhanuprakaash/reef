import { useState } from 'react';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';
import {
  builtinTools,
  customTools,
  mcpTool,
  toggleTool,
  toolHints,
  unrestrictedBashBlocked,
} from '@/modules/teams/utils/tools';

type Props = {
  label: string;
  tools: string[];
  write: boolean;
  servers: { name: string; label: string }[];
  onChange: (tools: string[]) => void;
};

const switchClass =
  'relative h-6 w-10 shrink-0 appearance-none rounded-pill bg-line outline-none transition-colors before:absolute before:top-[3px] before:left-[3px] before:size-[18px] before:rounded-full before:bg-soft before:transition-[left] checked:bg-done checked:before:left-[19px] checked:before:bg-fg focus-visible:ring-3 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-40';

const suggestions = [
  'Bash(git status:*)',
  'Bash(git diff:*)',
  'Bash(git log:*)',
  'Bash(go test:*)',
  'Bash(npm test:*)',
];

export function ToolPicker({ label, tools, write, servers, onChange }: Props) {
  const [draft, setDraft] = useState('');
  const entries = [
    ...builtinTools.map((tool) => ({ tool, hint: toolHints[tool] })),
    ...servers.map((s) => ({
      tool: mcpTool(s.name),
      hint: s.label ? `MCP server · ${s.label}` : 'MCP server',
    })),
  ];
  const custom = customTools(
    tools,
    servers.map((s) => s.name),
  );
  const value = draft.trim();
  const canAdd = value !== '' && !tools.includes(value);
  const quick = suggestions.filter((p) => !tools.includes(p));

  function add(tool: string) {
    onChange(toggleTool(tools, tool, true));
    setDraft('');
  }

  return (
    <div className="min-w-0 space-y-3">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-[11px] uppercase tracking-[0.08em] text-muted">Tools</h3>
        <span className="text-xs text-muted">{tools.length} allowed</span>
      </div>
      <div>
        {entries.map(({ tool, hint }) => {
          const blocked = unrestrictedBashBlocked(tool, write, tools);
          return (
            <label
              key={tool}
              className="flex min-h-11 items-center gap-3 border-b border-hairline last:border-b-0"
              title={
                blocked
                  ? "Read-only roles can't run every command. Add a pattern like Bash(git log:*)."
                  : undefined
              }
            >
              <span className={`min-w-0 flex-1 ${blocked ? 'opacity-60' : ''}`}>
                <span className="block truncate font-mono text-[13px]">{tool}</span>
                <span className="block truncate text-xs text-muted">{hint}</span>
              </span>
              <input
                type="checkbox"
                role="switch"
                className={switchClass}
                aria-label={`${label} tool ${tool}`}
                disabled={blocked}
                checked={tools.includes(tool)}
                onChange={(e) => onChange(toggleTool(tools, tool, e.target.checked))}
              />
            </label>
          );
        })}
      </div>
      <div className="space-y-1">
        <h3 className="text-[11px] uppercase tracking-[0.08em] text-muted">Custom patterns</h3>
        <p className="text-xs text-muted">Allow one command, e.g. Bash(git log:*)</p>
      </div>
      {custom.length > 0 && (
        <div>
          {custom.map((tool) => (
            <div
              key={tool}
              className="flex min-h-11 items-center gap-3 border-b border-hairline last:border-b-0"
            >
              <span className="min-w-0 flex-1">
                <span className="block truncate font-mono text-[13px]" title={tool}>
                  {tool}
                </span>
                <span className="block truncate text-xs text-muted">Custom pattern</span>
              </span>
              <button
                type="button"
                aria-label={`Remove ${tool}`}
                onClick={() => onChange(toggleTool(tools, tool, false))}
                className="flex size-7 shrink-0 items-center justify-center rounded-full text-muted hover:bg-hover hover:text-fg"
              >
                <svg
                  className="size-3"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2.6"
                  strokeLinecap="round"
                  aria-hidden="true"
                >
                  <path d="M6 6l12 12M18 6L6 18" />
                </svg>
              </button>
            </div>
          ))}
        </div>
      )}
      <div className="flex items-center gap-2">
        <TextField
          className="min-w-0 flex-1 font-mono"
          aria-label={`${label} add tool`}
          placeholder="Bash(go test:*)"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== 'Enter') return;
            e.preventDefault();
            if (canAdd) add(value);
          }}
        />
        <Button type="button" size="sm" disabled={!canAdd} onClick={() => add(value)}>
          Add
        </Button>
      </div>
      {quick.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-muted">Quick add</span>
          {quick.map((p) => (
            <button
              key={p}
              type="button"
              aria-label={`Add ${p}`}
              onClick={() => add(p)}
              className="h-[26px] rounded-pill bg-track px-2.5 font-mono text-xs text-soft hover:bg-hover hover:text-fg"
            >
              {p}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
