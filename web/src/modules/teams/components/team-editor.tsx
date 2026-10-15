import { useRef, useState } from 'react';
import { useUpdateTeam } from '@/modules/teams/api/update-team';
import { AddGateRow } from '@/modules/teams/components/add-gate-row';
import { AddRoleRow } from '@/modules/teams/components/add-role-row';
import { RoleEditor } from '@/modules/teams/components/role-editor';
import type { Gate, Role, Team } from '@/types/api';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';
import { Toggle } from '@/components/toggle';

type Props = { team: Team; onSaved: (name: string) => void; onDuplicate: () => void };

export function TeamEditor({ team, onSaved, onDuplicate }: Props) {
  const [roles, setRoles] = useState<Role[]>(team.roles);
  const nextKey = useRef(team.roles.length);
  const [keys, setKeys] = useState(() => team.roles.map((_, i) => i));
  const [gates, setGates] = useState<Gate[]>(team.gates ?? []);
  const [open, setOpen] = useState<number | null>(null);
  const save = useUpdateTeam();

  function updateRole(i: number, role: Role) {
    setRoles(roles.map((r, idx) => (idx === i ? role : r)));
  }
  function removeRole(i: number) {
    setRoles(roles.filter((_, idx) => idx !== i));
    setKeys(keys.filter((_, idx) => idx !== i));
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
        <div className="mb-4 flex flex-wrap items-end gap-4">
          <label className="block flex-1 basis-60">
            <span className="mb-1.5 block text-xs text-muted">Team name</span>
            <TextField className="w-full" readOnly value={team.name} />
          </label>
          <p className="pb-2.5 text-xs text-muted">
            Lead answers by default. Specialists sleep until @mentioned.
          </p>
        </div>
        <h2 className="text-[11px] uppercase tracking-[0.08em] text-muted">Roles</h2>
        {roles.map((role, i) => (
          <RoleEditor
            key={keys[i]}
            role={role}
            open={open === i}
            onToggle={() => setOpen(open === i ? null : i)}
            onChange={(r) => updateRole(i, r)}
            onRemove={() => removeRole(i)}
          />
        ))}
        <AddRoleRow
          onAdd={(role) => {
            setRoles([...roles, role]);
            setKeys([...keys, nextKey.current++]);
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
              key={`${gate.from}-${gate.to}`}
              className="flex min-h-11 items-center gap-2 border-b border-hairline text-[13px] text-soft last:border-b-0"
            >
              <span className="min-w-0 flex-1 truncate">
                {gate.from} → {gate.to}
              </span>
              <Toggle
                label={`Gate ${gate.from} to ${gate.to}`}
                checked={gate.on}
                onChange={(on) => updateGate(i, { ...gate, on })}
              />
              <Button
                variant="ghost"
                size="sm"
                type="button"
                aria-label={`Remove gate ${gate.from} to ${gate.to}`}
                onClick={() => removeGate(i)}
              >
                Remove
              </Button>
            </div>
          ))}
          <AddGateRow
            roles={roles.map((r) => r.name).filter(Boolean)}
            gates={gates}
            onAdd={(gate) => setGates([...gates, gate])}
          />
        </div>

        <div className="mt-auto space-y-2">
          <div className="flex gap-2">
            <Button className="flex-1" type="button" onClick={onDuplicate}>
              Duplicate
            </Button>
            <Button
              className="flex-1"
              variant="primary"
              type="button"
              disabled={save.isPending}
              onClick={() =>
                save.mutate(
                  { name: team.name, team: { name: team.name, roles, gates } },
                  {
                    onSuccess: (saved) => {
                      setRoles(saved.roles);
                      onSaved(saved.name);
                    },
                  },
                )
              }
            >
              Save team
            </Button>
          </div>
          {save.isSuccess && <p className="text-sm text-muted">Saved</p>}
          {save.isError && <p className="text-sm text-danger">{save.error.message}</p>}
        </div>
      </aside>
    </>
  );
}
