import type { Role } from '@/types/api';
import { buttonClass, inputClass, mutedClass } from '@/utils/classes';

type Props = { role: Role; onChange: (role: Role) => void; onRemove: () => void };

export function RoleEditor({ role, onChange, onRemove }: Props) {
  const label = role.name || 'role';
  return (
    <fieldset className="space-y-2 rounded border border-neutral-300 p-3 dark:border-neutral-700">
      <div className="flex items-center gap-2">
        <input
          className={`${inputClass} flex-1`}
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
        <button type="button" className={buttonClass} onClick={onRemove}>
          Remove
        </button>
      </div>
      <div className="flex gap-2">
        <input
          className={`${inputClass} flex-1`}
          aria-label={`${label} backend`}
          placeholder="backend"
          value={role.backend}
          onChange={(e) => onChange({ ...role, backend: e.target.value })}
        />
        <input
          className={`${inputClass} flex-1`}
          aria-label={`${label} permission mode`}
          placeholder="permission mode"
          value={role.permissionMode}
          onChange={(e) => onChange({ ...role, permissionMode: e.target.value })}
        />
      </div>
      <label className="block space-y-1">
        <span className={mutedClass}>Persona</span>
        <textarea
          className={`${inputClass} w-full`}
          rows={3}
          aria-label={`${label} persona`}
          value={role.persona}
          onChange={(e) => onChange({ ...role, persona: e.target.value })}
        />
      </label>
      <label className="block space-y-1">
        <span className={mutedClass}>Skills</span>
        <input
          className={`${inputClass} w-full`}
          aria-label={`${label} skills`}
          value={role.skills}
          onChange={(e) => onChange({ ...role, skills: e.target.value })}
        />
      </label>
      <label className="block space-y-1">
        <span className={mutedClass}>Tools (one per line)</span>
        <textarea
          className={`${inputClass} w-full`}
          rows={3}
          aria-label={`${label} tools`}
          value={role.tools.join('\n')}
          onChange={(e) =>
            onChange({
              ...role,
              tools: e.target.value
                .split('\n')
                .map((t) => t.trim())
                .filter(Boolean),
            })
          }
        />
      </label>
    </fieldset>
  );
}
