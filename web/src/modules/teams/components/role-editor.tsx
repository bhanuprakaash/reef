import type { Role } from '@/types/api';
import { ChipList } from '@/modules/teams/components/chip-list';
import { CreaturePicker } from '@/modules/teams/components/creature-picker';
import { backendLabel, backendOptions } from '@/modules/teams/utils/backend-label';
import { Button } from '@/components/button';
import { Creature, creatureName } from '@/components/creature';
import { Select } from '@/components/select';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

type Props = {
  role: Role;
  open: boolean;
  onToggle: () => void;
  onChange: (role: Role) => void;
  onRemove: () => void;
};

const modes: Record<string, string[]> = {
  claude: ['default', 'acceptEdits'],
  codex: ['read-only', 'workspace-write'],
};

function modeOptions(role: Role) {
  const values = ['', ...(modes[role.backend] ?? [])];
  if (!values.includes(role.permissionMode)) values.push(role.permissionMode);
  return values.map((value) => ({ value, label: value || 'Backend default' }));
}

const claudeModels = [
  { value: '', label: 'Default' },
  { value: 'opus', label: 'Opus' },
  { value: 'sonnet', label: 'Sonnet' },
  { value: 'haiku', label: 'Haiku' },
];

function claudeModelOptions(model: string) {
  return claudeModels.some((o) => o.value === model)
    ? claudeModels
    : [...claudeModels, { value: model, label: model }];
}

const badge = 'inline-flex h-[30px] items-center rounded-pill border px-3 text-xs';

export function RoleEditor({ role, open, onToggle, onChange, onRemove }: Props) {
  const label = role.name || 'role';
  return (
    <fieldset className="min-w-0 overflow-hidden rounded-2xl border border-line bg-card">
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="flex w-full flex-wrap items-center gap-3.5 px-4 py-3.5 text-left"
      >
        <Creature role={role.name} face={role.face} size={40} animate />
        <span className="min-w-0 flex-1 basis-40">
          <span className="flex items-center gap-2">
            <span>{role.name || 'role'}</span>
            {role.lead && (
              <span className="rounded-pill bg-lead px-2 text-[11px] leading-5 text-mark-ink">
                answers by default
              </span>
            )}
          </span>
          <span className="block truncate text-[12.5px] text-muted">{role.persona}</span>
        </span>
        <span className={`${badge} border-line text-soft`}>{backendLabel(role.backend)}</span>
        <span
          className={`${badge} ${role.write ? 'border-transparent bg-developer/15 text-developer' : 'border-line text-muted'}`}
        >
          {role.write ? 'edits code' : 'read-only'}
        </span>
      </button>
      {open && (
        <div className="space-y-3.5 border-t border-hairline bg-field p-4">
          <div className="flex flex-wrap items-center gap-3">
            <CreaturePicker
              value={creatureName(role.name, role.face)}
              onChange={(face) => onChange({ ...role, face })}
            />
            <TextField
              className="flex-1 basis-40"
              aria-label={`${label} name`}
              value={role.name}
              onChange={(e) => onChange({ ...role, name: e.target.value })}
            />
            <Select
              className="w-40"
              aria-label={`${label} backend`}
              value={role.backend}
              options={backendOptions}
              onChange={(backend) =>
                onChange({
                  ...role,
                  backend,
                  permissionMode: (modes[backend] ?? []).includes(role.permissionMode)
                    ? role.permissionMode
                    : '',
                  model: '',
                })
              }
            />
            <label className="flex items-center gap-1.5 text-sm">
              <input
                type="checkbox"
                aria-label={`${label} lead`}
                checked={role.lead}
                onChange={(e) => onChange({ ...role, lead: e.target.checked })}
              />
              Lead
            </label>
            <label className="flex items-center gap-1.5 text-sm">
              <input
                type="checkbox"
                aria-label={`${label} write`}
                checked={role.write}
                onChange={(e) => onChange({ ...role, write: e.target.checked })}
              />
              Write
            </label>
            <Button variant="ghost" type="button" onClick={onRemove}>
              Remove
            </Button>
          </div>
          <label className="block space-y-1.5">
            <span className="text-xs text-muted">Persona</span>
            <TextArea
              className="w-full leading-normal"
              rows={3}
              aria-label={`${label} persona`}
              value={role.persona}
              onChange={(e) => onChange({ ...role, persona: e.target.value })}
            />
          </label>
          <label className="block space-y-1.5">
            <span className="text-xs text-muted">Skills</span>
            <TextArea
              className="w-full leading-normal"
              rows={2}
              aria-label={`${label} skills`}
              value={role.skills}
              onChange={(e) => onChange({ ...role, skills: e.target.value })}
            />
          </label>
          <div className="flex flex-wrap gap-x-6 gap-y-3.5">
            <ChipList
              label="Allowed tools"
              hint="Claude only"
              mono
              values={role.tools}
              addLabel={`${label} add tool`}
              onAdd={(tool) => onChange({ ...role, tools: [...role.tools, tool] })}
              onRemove={(i) => onChange({ ...role, tools: role.tools.filter((_, j) => j !== i) })}
            />
            <div className="min-w-0">
              <div className="mb-2 text-xs text-muted">Permission mode</div>
              <Select
                variant="pill"
                aria-label={`${label} permission mode`}
                value={role.permissionMode}
                options={modeOptions(role)}
                onChange={(permissionMode) => onChange({ ...role, permissionMode })}
              />
            </div>
            <div className="min-w-0">
              <div className="mb-2 text-xs text-muted">Model</div>
              {role.backend === 'claude' ? (
                <Select
                  variant="pill"
                  aria-label={`${label} model`}
                  value={role.model ?? ''}
                  options={claudeModelOptions(role.model ?? '')}
                  onChange={(model) => onChange({ ...role, model })}
                />
              ) : (
                <TextField
                  className="w-40"
                  aria-label={`${label} model`}
                  placeholder="Default"
                  value={role.model ?? ''}
                  onChange={(e) => onChange({ ...role, model: e.target.value })}
                />
              )}
            </div>
            <ChipList
              label="MCP servers"
              empty="None. Add them with reef team apply."
              values={role.mcp.map((m) => (m.label ? `${m.name} · ${m.label}` : m.name))}
              onRemove={(i) =>
                onChange({
                  ...role,
                  mcp: role.mcp.filter((_, j) => j !== i),
                  mcpRemove: [...(role.mcpRemove ?? []), role.mcp[i]],
                })
              }
            />
          </div>
        </div>
      )}
    </fieldset>
  );
}
