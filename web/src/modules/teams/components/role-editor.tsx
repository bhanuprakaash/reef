import { useState } from 'react';
import type { Role } from '@/types/api';
import { parseTools } from '@/modules/teams/utils/parse-tools';
import { Button } from '@/components/button';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

type Props = { role: Role; onChange: (role: Role) => void; onRemove: () => void };

export function RoleEditor({ role, onChange, onRemove }: Props) {
  const label = role.name || 'role';
  const [toolsText, setToolsText] = useState(role.tools.join('\n'));
  return (
    <fieldset className="space-y-2 rounded border border-line p-3">
      <div className="flex items-center gap-2">
        <TextField
          className="flex-1"
          aria-label={`${label} name`}
          value={role.name}
          onChange={(e) => onChange({ ...role, name: e.target.value })}
        />
        <label className="flex items-center gap-1 text-sm">
          <input
            type="checkbox"
            aria-label={`${label} lead`}
            checked={role.lead}
            onChange={(e) => onChange({ ...role, lead: e.target.checked })}
          />
          Lead
        </label>
        <label className="flex items-center gap-1 text-sm">
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
      <div className="flex gap-2">
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
      <label className="block space-y-1">
        <span className="text-sm text-muted">Persona</span>
        <TextArea
          className="w-full"
          rows={3}
          aria-label={`${label} persona`}
          value={role.persona}
          onChange={(e) => onChange({ ...role, persona: e.target.value })}
        />
      </label>
      <label className="block space-y-1">
        <span className="text-sm text-muted">Skills</span>
        <TextField
          className="w-full"
          aria-label={`${label} skills`}
          value={role.skills}
          onChange={(e) => onChange({ ...role, skills: e.target.value })}
        />
      </label>
      <label className="block space-y-1">
        <span className="text-sm text-muted">Tools (one per line)</span>
        <TextArea
          className="w-full"
          rows={3}
          aria-label={`${label} tools`}
          value={toolsText}
          onChange={(e) => setToolsText(e.target.value)}
          onBlur={() => onChange({ ...role, tools: parseTools(toolsText) })}
        />
      </label>
    </fieldset>
  );
}
