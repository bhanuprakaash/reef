import { useState } from 'react';
import type { Role } from '@/types/api';
import { CreaturePicker } from '@/modules/teams/components/creature-picker';
import { backendLabel } from '@/modules/teams/utils/backend-label';
import { parseTools } from '@/modules/teams/utils/parse-tools';
import { Button } from '@/components/button';
import { Chip } from '@/components/chip';
import { Creature, creatureName } from '@/components/creature';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

type Props = {
  role: Role;
  open: boolean;
  onToggle: () => void;
  onChange: (role: Role) => void;
  onRemove: () => void;
};

export function RoleEditor({ role, open, onToggle, onChange, onRemove }: Props) {
  const label = role.name || 'role';
  const [toolsText, setToolsText] = useState(role.tools.join('\n'));
  return (
    <fieldset className="min-w-0 overflow-hidden rounded-2xl border border-line bg-card">
      <button
        type="button"
        aria-expanded={open}
        onClick={onToggle}
        className="flex w-full flex-wrap items-center gap-3.5 px-4 py-3.5 text-left"
      >
        <Creature role={role.name} face={role.face} size={40} />
        <span className="min-w-0 flex-1 basis-40">
          <span className="flex items-center gap-2">
            <span className="font-semibold">{role.name || 'role'}</span>
            {role.lead && (
              <span className="rounded-pill bg-lead/15 px-2 text-[11px] leading-5 text-lead">
                answers by default
              </span>
            )}
          </span>
          <span className="block truncate text-[12.5px] text-muted">{role.persona}</span>
        </span>
        <Chip>{backendLabel(role.backend)}</Chip>
        <Chip>{role.write ? 'edits code' : 'read-only'}</Chip>
      </button>
      {open && (
        <div className="space-y-3.5 border-t border-hairline bg-field p-4">
          <div className="flex items-center gap-3">
            <TextField
              className="flex-1"
              aria-label={`${label} name`}
              value={role.name}
              onChange={(e) => onChange({ ...role, name: e.target.value })}
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
          <div className="space-y-1.5">
            <span className="text-xs text-muted">Creature</span>
            <CreaturePicker
              value={creatureName(role.name, role.face)}
              onChange={(face) => onChange({ ...role, face })}
            />
          </div>
          <div className="flex gap-3">
            <TextField
              className="flex-1"
              aria-label={`${label} backend`}
              placeholder="backend"
              value={role.backend}
              onChange={(e) => onChange({ ...role, backend: e.target.value })}
            />
            <TextField
              className="flex-1"
              aria-label={`${label} permission mode`}
              placeholder="permission mode"
              value={role.permissionMode}
              onChange={(e) => onChange({ ...role, permissionMode: e.target.value })}
            />
          </div>
          <label className="block space-y-1.5">
            <span className="text-xs text-muted">Persona</span>
            <TextArea
              className="w-full"
              rows={3}
              aria-label={`${label} persona`}
              value={role.persona}
              onChange={(e) => onChange({ ...role, persona: e.target.value })}
            />
          </label>
          <label className="block space-y-1.5">
            <span className="text-xs text-muted">Skills</span>
            <TextField
              className="w-full"
              aria-label={`${label} skills`}
              value={role.skills}
              onChange={(e) => onChange({ ...role, skills: e.target.value })}
            />
          </label>
          <label className="block space-y-1.5">
            <span className="text-xs text-muted">Allowed tools (one per line)</span>
            <TextArea
              className="w-full font-mono"
              rows={3}
              aria-label={`${label} tools`}
              value={toolsText}
              onChange={(e) => setToolsText(e.target.value)}
              onBlur={() => onChange({ ...role, tools: parseTools(toolsText) })}
            />
          </label>
        </div>
      )}
    </fieldset>
  );
}
