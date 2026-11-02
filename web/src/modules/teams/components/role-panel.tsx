import { useEffect, type ReactNode } from 'react';
import { Link } from 'react-router';
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
import { claudeModelOptions } from '@/utils/models';

type Props = {
  role: Role;
  skills?: Skills;
  connections?: Connection[];
  providers?: Provider[];
  noProject?: boolean;
  onChange: (role: Role) => void;
  onClose: () => void;
  extra?: ReactNode;
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

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-2">
      <h3 className="text-[11px] uppercase tracking-[0.08em] text-muted">{title}</h3>
      {children}
    </section>
  );
}

function Group({ children }: { children: ReactNode }) {
  return <div className="divide-y divide-hairline rounded-[14px] bg-field px-3.5">{children}</div>;
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
  extra,
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
      className="fixed inset-y-3 right-3 z-30 flex w-[calc(100%-24px)] max-w-[420px] flex-col overflow-hidden rounded-card border border-line bg-card shadow-dialog"
    >
      <div className="flex-1 space-y-5 overflow-y-auto p-5">
        <div className="flex items-start gap-3.5">
          <CreaturePicker
            size={56}
            value={role}
            onChange={(look) => onChange({ ...role, ...look })}
          />
          <div className="min-w-0 flex-1">
            <TextField
              className="w-full border-transparent! bg-transparent! text-lg hover:border-line! hover:bg-field! focus-visible:border-fg!"
              aria-label={`${label} name`}
              placeholder="Name"
              value={role.name}
              onChange={(e) => onChange({ ...role, name: e.target.value })}
            />
            <div className="mt-1 flex items-center gap-3 px-3 text-[13px] text-muted">
              <span className="min-w-0 flex-1 truncate">
                {role.description.split('\n')[0] || 'No description yet'}
              </span>
              <Link to={`/bots/${role.botId}`} className="shrink-0 text-soft">
                Open profile
              </Link>
            </div>
          </div>
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

        <Section title="About">
          <TextArea
            className="w-full leading-normal"
            rows={5}
            aria-label={`${label} description`}
            placeholder="What this Bot does, in plain words."
            value={role.description}
            onChange={(e) => onChange({ ...role, description: e.target.value })}
          />
        </Section>

        <Section title="Runtime">
          <Group>
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
            <SettingRow title="Permission mode">
              <Select
                variant="pill"
                aria-label={`${label} permission mode`}
                value={role.permissionMode}
                options={modeOptions(role)}
                onChange={(permissionMode) => onChange({ ...role, permissionMode })}
              />
            </SettingRow>
            <SettingRow title="Can edit code" hint="Off keeps the Bot read-only.">
              <Toggle
                label={`${label} write`}
                checked={role.write}
                onChange={(write) => onChange({ ...role, write })}
              />
            </SettingRow>
          </Group>
        </Section>

        <Section title="Access">
          <Group>
            {role.backend === 'claude' && (
              <div className="py-2.5">
                <ToolPicker
                  label={label}
                  tools={role.tools}
                  write={role.write}
                  servers={(connections ?? [])
                    .filter((c) => role.connections.includes(c.id))
                    .map((c) => ({ name: c.name, label: connectionLabel(c) }))}
                  onChange={(tools) => onChange({ ...role, tools })}
                />
              </div>
            )}
            <div className="py-2.5">
              <ConnectionPicker
                label={label}
                selected={role.connections}
                available={connections}
                onChange={(next) => onChange({ ...role, connections: next })}
              />
            </div>
            <div className="py-2.5">
              <SkillPicker
                label={label}
                picked={role.skills}
                available={skills}
                backend={role.backend}
                noProject={noProject}
                limits={role.backend === 'claude'}
                onChange={(next) => onChange({ ...role, skills: next })}
              />
            </div>
          </Group>
        </Section>

        {extra}
      </div>
      <footer className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-hairline px-5 py-3.5">
        {children}
      </footer>
    </aside>
  );
}
