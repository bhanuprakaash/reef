import { useState } from 'react';
import { useUpdateTeam } from '@/modules/teams/api/update-team';
import { RoleEditor } from '@/modules/teams/components/role-editor';
import type { Gate, Role, Team } from '@/types/api';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';

const teamNamePattern = /^[A-Za-z0-9_-]+$/;

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
  const save = useUpdateTeam();
  const copy = useUpdateTeam();

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
        <Button type="button" onClick={() => setRoles([...roles, { ...blankRole }])}>
          Add role
        </Button>
      </div>

      <div className="space-y-2">
        <h3 className="font-medium">Gates</h3>
        {gates.map((gate, i) => (
          <div key={i} className="flex items-center gap-2">
            <TextField
              aria-label={`gate ${i} from`}
              placeholder="from"
              value={gate.from}
              onChange={(e) => updateGate(i, { ...gate, from: e.target.value })}
            />
            <TextField
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
            <Button variant="ghost" type="button" onClick={() => removeGate(i)}>
              Remove
            </Button>
          </div>
        ))}
        <Button type="button" onClick={() => setGates([...gates, { from: '', to: '', on: true }])}>
          Add gate
        </Button>
      </div>

      <div className="flex items-center gap-3">
        <Button
          type="button"
          disabled={save.isPending}
          onClick={() =>
            save.mutate(
              { name: team.name, team: { name: team.name, roles, gates } },
              { onSuccess: (saved) => onSaved(saved.name) },
            )
          }
        >
          Save
        </Button>
        {save.isSuccess && <span className="text-sm text-muted">Saved</span>}
        {save.isError && <span className="text-sm text-danger">{save.error.message}</span>}
      </div>

      <div className="flex items-center gap-2">
        <TextField
          aria-label="New team name"
          placeholder="new team name"
          value={copyName}
          onChange={(e) => setCopyName(e.target.value)}
        />
        <Button
          type="button"
          disabled={copy.isPending || !teamNamePattern.test(copyName.trim())}
          onClick={() =>
            copy.mutate(
              {
                name: copyName.trim(),
                team: { name: copyName.trim(), roles, gates },
                from: team.name,
              },
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
        </Button>
        {copy.isSuccess && <span className="text-sm text-muted">Saved</span>}
        {copy.isError && <span className="text-sm text-danger">{copy.error.message}</span>}
      </div>
    </div>
  );
}
