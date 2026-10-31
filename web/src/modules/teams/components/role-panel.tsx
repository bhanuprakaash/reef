import { useEffect, type ReactNode } from 'react';
import type { Connection, Provider, Role, Skills } from '@/types/api';
import { ConnectionPicker } from '@/modules/teams/components/connection-picker';
import { connectionLabel } from '@/modules/teams/utils/connections';
import { CreaturePicker } from '@/modules/teams/components/creature-picker';
import { SkillPicker } from '@/modules/teams/components/skill-picker';
import { ToolPicker } from '@/modules/teams/components/tool-picker';
import { providerOptions, providerFor } from '@/modules/teams/utils/providers';
import { backendOptions } from '@/modules/teams/utils/backend-label';
import { Select } from '@/components/select';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';
import { Toggle } from '@/components/toggle';
import { claudeModels } from '@/utils/models';

type Props = {
  role: Role;
  skills?: Skills;
  connections?: Connection[];
  providers?: Provider[];
  noProject?: boolean;
  onChange: (role: Role) => void;
  onClose: () => void;
  children: ReactNode;
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

export function RolePanel({
  role,
  skills,
  connections,
  providers = [],
  noProject,
  onChange,
  onClose,
  children,
}: Props) {
  const label = role.name || 'role';

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  return (
    <aside
      aria-label="Bot"
      className="fixed inset-y-3 right-3 z-30 w-[calc(100%-24px)] max-w-[420px] space-y-[18px] overflow-y-auto rounded-card border border-line bg-card p-5 shadow-dialog"
    >
      <div className="space-y-3">
        <div className="flex items-center gap-3">
          <CreaturePicker
            size={56}
            value={role}
            onChange={(look) => onChange({ ...role, ...look })}
          />
          <TextField
            className="min-w-0 flex-1"
            aria-label={`${label} name`}
            placeholder="Name"
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
        <SettingRow title="Can edit code" hint="Off keeps the Bot read-only.">
          <Toggle
            label={`${label} write`}
            checked={role.write}
            onChange={(write) => onChange({ ...role, write })}
          />
        </SettingRow>
      </div>

      <Section title="Describe this Bot">
        <p className="text-xs text-muted">What this Bot does, in plain words.</p>
        <TextArea
          className="w-full leading-normal"
          rows={7}
          aria-label={`${label} description`}
          value={role.description}
          onChange={(e) => onChange({ ...role, description: e.target.value })}
        />
      </Section>

      <Section title="Runtime">
        <>
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
                  provider: providerFor(role.provider, backend, providers),
                  permissionMode: (modes[backend] ?? []).includes(role.permissionMode)
                    ? role.permissionMode
                    : '',
                  model: '',
                })
              }
            />
          </SettingRow>
          <SettingRow title="Provider" hint="Add installs in Settings.">
            <Select
              variant="pill"
              aria-label={`${label} provider`}
              value={role.provider ?? ''}
              options={providerOptions(role, providers)}
              onChange={(provider) => onChange({ ...role, provider: provider || undefined })}
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
        </>
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
            servers={(connections ?? [])
              .filter((c) => role.connections.includes(c.id))
              .map((c) => ({ name: c.name, label: connectionLabel(c) }))}
            onChange={(tools) => onChange({ ...role, tools })}
          />
        </Section>
      )}

      <Section>
        <ConnectionPicker
          label={label}
          selected={role.connections}
          available={connections}
          onChange={(next) => onChange({ ...role, connections: next })}
        />
      </Section>

      <Section>
        <SkillPicker
          label={label}
          picked={role.skills}
          available={skills}
          noProject={noProject}
          limits={role.backend === 'claude'}
          onChange={(next) => onChange({ ...role, skills: next })}
        />
      </Section>

      <Section>{children}</Section>
    </aside>
  );
}
