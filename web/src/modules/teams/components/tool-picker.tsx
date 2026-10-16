import { ChipList } from '@/modules/teams/components/chip-list';
import {
  builtinTools,
  customTools,
  mcpTool,
  toggleTool,
  unrestrictedBashBlocked,
} from '@/modules/teams/utils/tools';

type Props = {
  label: string;
  tools: string[];
  write: boolean;
  servers: { name: string; label: string }[];
  onChange: (tools: string[]) => void;
};

export function ToolPicker({ label, tools, write, servers, onChange }: Props) {
  const entries = [
    ...builtinTools.map((tool) => ({ tool, text: tool })),
    ...servers.map((s) => ({
      tool: mcpTool(s.name),
      text: s.label ? `${s.name} · ${s.label}` : s.name,
    })),
  ];
  const custom = customTools(
    tools,
    servers.map((s) => s.name),
  );
  return (
    <div className="min-w-0 basis-full">
      <div className="mb-2 text-xs text-muted">Allowed tools</div>
      <div className="mb-3 flex flex-wrap gap-x-4 gap-y-1">
        {entries.map(({ tool, text }) => {
          const blocked = unrestrictedBashBlocked(tool, write, tools);
          return (
            <label
              key={tool}
              className="flex items-center gap-x-1.5 text-sm"
              title={
                blocked
                  ? "Read-only roles can't run every command. Add a pattern like Bash(git log:*)."
                  : undefined
              }
            >
              <input
                type="checkbox"
                aria-label={`${label} tool ${tool}`}
                disabled={blocked}
                checked={tools.includes(tool)}
                onChange={(e) => onChange(toggleTool(tools, tool, e.target.checked))}
              />
              {text}
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
