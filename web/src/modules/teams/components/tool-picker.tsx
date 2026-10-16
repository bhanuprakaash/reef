import { ChipList } from '@/modules/teams/components/chip-list';
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

export function ToolPicker({ label, tools, write, servers, onChange }: Props) {
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
      <ChipList
        label="Custom tools"
        hint="a pattern such as Bash(git log:*)"
        mono
        values={custom}
        addLabel={`${label} add tool`}
        onAdd={(tool) => onChange(toggleTool(tools, tool, true))}
        onRemove={(i) => onChange(toggleTool(tools, custom[i], false))}
      />
    </div>
  );
}
