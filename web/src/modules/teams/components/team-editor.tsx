import { useState } from 'react';
import { useUpdateTeam } from '@/modules/teams/api/update-team';
import { RoleEditor } from '@/modules/teams/components/role-editor';
import type { Gate, Role, Team } from '@/types/api';
import { buttonClass, errorClass, inputClass, mutedClass } from '@/utils/classes';

const blankRole: Role = {
  name: '',
  lead: false,
  write: false,
  backend: 'claude',
  permissionMode: '',
  persona: '',
  skills: '',
  tools: [],
};

type Props = { team: Team; onSaved: (name: string) => void };

export function TeamEditor({ team, onSaved }: Props) {
  const [roles, setRoles] = useState<Role[]>(team.roles);
  const [gates, setGates] = useState<Gate[]>(team.gates ?? []);
  const [copyName, setCopyName] = useState('');
  const update = useUpdateTeam();

  function updateRole(i: number, role: Role) {
    setRoles(roles.map((r, idx) => (idx === i ? role : r)));
  }
  function removeRole(i: number) {
    setRoles(roles.filter((_, idx) => idx !== i));
  }
  function updateGate(i: number, gate: Gate) {
    setGates(gates.map((g, idx) => (idx === i ? gate : g)));
  }
  function removeGate(i: number) {
    setGates(gates.filter((_, idx) => idx !== i));
  }

  return (
    <div className="space-y-4">
      <div className="space-y-3">
        {roles.map((role, i) => (
          <RoleEditor
            key={i}
            role={role}
            onChange={(r) => updateRole(i, r)}
            onRemove={() => removeRole(i)}
          />
        ))}
        <button
          type="button"
          className={buttonClass}
          onClick={() => setRoles([...roles, { ...blankRole }])}
        >
          Add role
        </button>
      </div>

      <div className="space-y-2">
        <h3 className="font-medium">Gates</h3>
        {gates.map((gate, i) => (
          <div key={i} className="flex items-center gap-2">
            <input
              className={inputClass}
              aria-label={`gate ${i} from`}
              placeholder="from"
              value={gate.from}
              onChange={(e) => updateGate(i, { ...gate, from: e.target.value })}
            />
            <input
              className={inputClass}
              aria-label={`gate ${i} to`}
              placeholder="to"
              value={gate.to}
              onChange={(e) => updateGate(i, { ...gate, to: e.target.value })}
            />
            <label className="flex items-center gap-1 text-sm">
              <input
                type="checkbox"
                aria-label={`gate ${i} on`}
                checked={gate.on}
                onChange={(e) => updateGate(i, { ...gate, on: e.target.checked })}
              />
              On
            </label>
            <button type="button" className={buttonClass} onClick={() => removeGate(i)}>
              Remove
            </button>
          </div>
        ))}
        <button
          type="button"
          className={buttonClass}
          onClick={() => setGates([...gates, { from: '', to: '', on: true }])}
        >
          Add gate
        </button>
      </div>

      <div className="flex items-center gap-3">
        <button
          type="button"
          className={buttonClass}
          disabled={update.isPending}
          onClick={() =>
            update.mutate(
              { name: team.name, team: { name: team.name, roles, gates } },
              { onSuccess: (saved) => onSaved(saved.name) },
            )
          }
        >
          Save
        </button>
        {update.isError && <span className={errorClass}>{update.error.message}</span>}
      </div>

      <div className="flex items-center gap-2">
        <input
          className={inputClass}
          aria-label="New team name"
          placeholder="new team name"
          value={copyName}
          onChange={(e) => setCopyName(e.target.value)}
        />
        <button
          type="button"
          className={buttonClass}
          disabled={update.isPending || copyName.trim() === ''}
          onClick={() =>
            update.mutate(
              { name: copyName.trim(), team: { name: copyName.trim(), roles, gates } },
              {
                onSuccess: (saved) => {
                  setCopyName('');
                  onSaved(saved.name);
                },
              },
            )
          }
        >
          Copy as new team
        </button>
        {update.isSuccess && <span className={mutedClass}>Saved</span>}
      </div>
    </div>
  );
}
