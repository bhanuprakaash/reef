import type { ReactNode } from 'react';
import type { Role, Skills } from '@/types/api';
import { ChipList } from '@/modules/teams/components/chip-list';
import { CreaturePicker } from '@/modules/teams/components/creature-picker';
import { SkillPicker } from '@/modules/teams/components/skill-picker';
import { ToolPicker } from '@/modules/teams/components/tool-picker';
import { backendOptions } from '@/modules/teams/utils/backend-label';
import { Button } from '@/components/button';
import { creatureName } from '@/components/creature';
import { Select } from '@/components/select';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';
import { Toggle } from '@/components/toggle';

type Props = {
  role: Role;
  skills?: Skills;
  noProject?: boolean;
  onChange: (role: Role) => void;
  onRemove: () => void;
  onClose: () => void;
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

function Section({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <section className="space-y-3 border-t border-hairline pt-[18px]">
      {title && <h3 className="text-[11px] uppercase tracking-[0.08em] text-muted">{title}</h3>}
      {children}
    </section>
  );
}

function SettingRow({
  title,
  hint,
  children,
}: {
  title: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-11 items-center justify-between gap-3">
      <div className="min-w-0">
        <div className="text-sm">{title}</div>
        {hint && <div className="text-xs text-muted">{hint}</div>}
      </div>
      {children}
    </div>
  );
}

export function RolePanel({ role, skills, noProject, onChange, onRemove, onClose }: Props) {
  const label = role.name || 'role';
  return (
    <aside
      aria-label="Role"
      className="w-full space-y-[18px] rounded-card border border-line bg-card p-5 motion-safe:animate-rise-in lg:sticky lg:top-6 lg:max-h-[calc(100vh-48px)] lg:w-[400px] lg:overflow-y-auto"
    >
      <div className="space-y-3">
        <div className="flex items-center gap-3">
          <CreaturePicker
            size={56}
            value={creatureName(role.name, role.face)}
            onChange={(face) => onChange({ ...role, face })}
          />
          <TextField
            className="min-w-0 flex-1"
            aria-label={`${label} name`}
            value={role.name}
            onChange={(e) => onChange({ ...role, name: e.target.value })}
          />
          <button
            type="button"
            aria-label="Close role"
            onClick={onClose}
            className="flex size-9 shrink-0 items-center justify-center rounded-field text-soft hover:bg-hover"
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
        <SettingRow title="Answers by default" hint="Replies when nobody is @mentioned.">
          <Toggle
            label={`${label} lead`}
            checked={role.lead}
            onChange={(lead) => onChange({ ...role, lead })}
          />
        </SettingRow>
        <SettingRow title="Can edit code" hint="Off keeps the role read-only.">
          <Toggle
            label={`${label} write`}
            checked={role.write}
            onChange={(write) => onChange({ ...role, write })}
          />
        </SettingRow>
      </div>

      <Section title="Persona">
        <p className="text-xs text-muted">How this role thinks and works. Sent on every turn.</p>
        <TextArea
          className="w-full leading-normal"
          rows={7}
          aria-label={`${label} persona`}
          value={role.persona}
          onChange={(e) => onChange({ ...role, persona: e.target.value })}
        />
      </Section>

      <Section title="Runtime">
        <SettingRow title="Backend">
          <Select
            variant="pill"
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
        </SettingRow>
        <SettingRow title="Model">
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
        </SettingRow>
        <SettingRow title="Permission mode">
          <Select
            variant="pill"
            aria-label={`${label} permission mode`}
            value={role.permissionMode}
            options={modeOptions(role)}
            onChange={(permissionMode) => onChange({ ...role, permissionMode })}
          />
        </SettingRow>
      </Section>

      {role.backend === 'claude' && (
        <Section>
          <ToolPicker
            label={label}
            tools={role.tools}
            write={role.write}
            servers={role.mcp}
            onChange={(tools) => onChange({ ...role, tools })}
          />
        </Section>
      )}

      <Section>
        <SkillPicker
          label={label}
          picked={role.skills}
          available={role.backend === 'codex' ? skills?.codex : skills?.claude}
          noProject={noProject}
          limits={role.backend === 'claude'}
          onChange={(next) => onChange({ ...role, skills: next })}
        />
      </Section>

      <Section>
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
      </Section>

      <Section>
        <Button variant="ghost" type="button" className="text-danger!" onClick={onRemove}>
          Remove role
        </Button>
      </Section>
    </aside>
  );
}
