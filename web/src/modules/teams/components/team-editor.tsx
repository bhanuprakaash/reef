import { useState } from 'react';
import { useUpdateTeam } from '@/modules/teams/api/update-team';
import { AddRoleCard } from '@/modules/teams/components/add-role-card';
import { RoleEditor } from '@/modules/teams/components/role-editor';
import type { Gate, Role, Team } from '@/types/api';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';
import { Toggle } from '@/components/toggle';

const teamNamePattern = /^[A-Za-z0-9_-]+$/;

type Props = { team: Team; onSaved: (name: string) => void };

export function TeamEditor({ team, onSaved }: Props) {
  const [roles, setRoles] = useState<Role[]>(team.roles);
  const [gates, setGates] = useState<Gate[]>(team.gates ?? []);
  const [open, setOpen] = useState<number | null>(null);
  const [copyName, setCopyName] = useState('');
  const save = useUpdateTeam();
  const copy = useUpdateTeam();

  function updateRole(i: number, role: Role) {
    setRoles(roles.map((r, idx) => (idx === i ? role : r)));
  }
  function removeRole(i: number) {
    setRoles(roles.filter((_, idx) => idx !== i));
    setOpen(null);
  }
  function updateGate(i: number, gate: Gate) {
    setGates(gates.map((g, idx) => (idx === i ? gate : g)));
  }
  function removeGate(i: number) {
    setGates(gates.filter((_, idx) => idx !== i));
  }

  return (
    <>
      <section aria-label="Roles" className="min-w-0 flex-1 basis-80 space-y-2.5">
        <h2 className="text-[11px] uppercase tracking-[0.08em] text-muted">Roles</h2>
        {roles.map((role, i) => (
          <RoleEditor
            key={i}
            role={role}
            open={open === i}
            onToggle={() => setOpen(open === i ? null : i)}
            onChange={(r) => updateRole(i, r)}
            onRemove={() => removeRole(i)}
          />
        ))}
        <AddRoleCard
          onAdd={(role) => {
            setRoles([...roles, role]);
            setOpen(roles.length);
          }}
        />
      </section>

      <aside aria-label="Gates" className="flex w-full flex-col gap-6 lg:w-64">
        <div>
          <h2 className="mb-1 text-[11px] uppercase tracking-[0.08em] text-muted">
            Approval gates
          </h2>
          <p className="mb-3 text-[12.5px] text-muted">
            Work waits for you before it passes between these roles.
          </p>
          {gates.map((gate, i) => (
            <div
              key={i}
              className="flex min-h-11 items-center gap-2 border-b border-hairline last:border-b-0"
            >
              <TextField
                className="w-0 flex-1"
                aria-label={`gate ${i} from`}
                placeholder="from"
                value={gate.from}
                onChange={(e) => updateGate(i, { ...gate, from: e.target.value })}
              />
              <TextField
                className="w-0 flex-1"
                aria-label={`gate ${i} to`}
                placeholder="to"
                value={gate.to}
                onChange={(e) => updateGate(i, { ...gate, to: e.target.value })}
              />
              <Toggle
                label={`gate ${i} on`}
                checked={gate.on}
                onChange={(on) => updateGate(i, { ...gate, on })}
              />
              <Button variant="ghost" type="button" onClick={() => removeGate(i)}>
                Remove
              </Button>
            </div>
          ))}
          <Button
            className="mt-3"
            type="button"
            onClick={() => setGates([...gates, { from: '', to: '', on: true }])}
          >
            Add gate
          </Button>
        </div>

        <div className="space-y-2">
          <Button
            className="w-full"
            variant="primary"
            type="button"
            disabled={save.isPending}
            onClick={() =>
              save.mutate(
                { name: team.name, team: { name: team.name, roles, gates } },
                { onSuccess: (saved) => onSaved(saved.name) },
              )
            }
          >
            Save team
          </Button>
          {save.isSuccess && <p className="text-sm text-muted">Saved</p>}
          {save.isError && <p className="text-sm text-danger">{save.error.message}</p>}
        </div>

        <div className="space-y-2">
          <TextField
            className="w-full"
            aria-label="New team name"
            placeholder="new team name"
            value={copyName}
            onChange={(e) => setCopyName(e.target.value)}
          />
          <Button
            className="w-full"
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
          {copy.isSuccess && <p className="text-sm text-muted">Saved</p>}
          {copy.isError && <p className="text-sm text-danger">{copy.error.message}</p>}
        </div>
      </aside>
    </>
  );
}
