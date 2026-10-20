import { useState } from 'react';
import { useUpdateTeam } from '@/modules/teams/api/update-team';
import { AddGateRow } from '@/modules/teams/components/add-gate-row';
import { AddRoleRow } from '@/modules/teams/components/add-role-row';
import { RolePanel } from '@/modules/teams/components/role-panel';
import type { Gate, Role, Skills, Team } from '@/types/api';
import { backendLabel } from '@/modules/teams/utils/backend-label';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';
import { TextField } from '@/components/text-field';
import { Toggle } from '@/components/toggle';

const badge = 'inline-flex h-[30px] items-center rounded-pill border px-3 text-xs';

type Props = {
  team: Team;
  skills?: Skills;
  noProject?: boolean;
  onSaved: (name: string) => void;
  onDuplicate: () => void;
};

export function TeamEditor({ team, skills, noProject, onSaved, onDuplicate }: Props) {
  const [loaded, setLoaded] = useState(team);
  const [roles, setRoles] = useState<Role[]>(team.roles);
  const [keys, setKeys] = useState(() => team.roles.map((_, i) => i));
  const [gates, setGates] = useState<Gate[]>(team.gates ?? []);
  const [selected, setSelected] = useState<number | null>(null);
  const save = useUpdateTeam();

  function load(t: Team) {
    setLoaded(t);
    setRoles(t.roles);
    setGates(t.gates ?? []);
    setKeys(t.roles.map((_, i) => i));
  }
  const unsaved =
    JSON.stringify([roles, gates]) !== JSON.stringify([loaded.roles, loaded.gates ?? []]);
  if (team !== loaded && !unsaved) load(team);

  const current = selected === null ? undefined : roles[selected];

  function updateRole(i: number, role: Role) {
    setRoles(roles.map((r, idx) => (idx === i ? role : r)));
  }
  function removeRole(i: number) {
    setRoles(roles.filter((_, idx) => idx !== i));
    setKeys(keys.filter((_, idx) => idx !== i));
    setSelected(null);
  }
  function updateGate(i: number, gate: Gate) {
    setGates(gates.map((g, idx) => (idx === i ? gate : g)));
  }
  function removeGate(i: number) {
    setGates(gates.filter((_, idx) => idx !== i));
  }

  return (
    <>
      <div className="min-w-0 flex-1 basis-80 space-y-6">
        <div className="space-y-2.5">
          <div className="flex flex-wrap items-end gap-3">
            <label className="block flex-1 basis-60">
              <span className="mb-1.5 block text-xs text-muted">Team name</span>
              <TextField className="w-full" readOnly value={team.name} />
            </label>
            <p className="basis-48 pb-2.5 text-xs text-muted">
              Lead answers by default. Specialists sleep until @mentioned.
            </p>
            <Button type="button" onClick={onDuplicate}>
              Duplicate
            </Button>
            <Button
              variant="primary"
              type="button"
              disabled={save.isPending}
              onClick={() =>
                save.mutate(
                  { name: team.name, team: { name: team.name, roles, gates } },
                  {
                    onSuccess: (saved) => {
                      load(saved);
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

        <section aria-label="Roles" className="space-y-1.5">
          <h2 className="mb-2.5 text-[11px] uppercase tracking-[0.08em] text-muted">Roles</h2>
          {roles.map((role, i) => (
            <button
              key={keys[i]}
              type="button"
              aria-pressed={selected === i}
              onClick={() => setSelected(i)}
              className={`flex w-full flex-wrap items-center gap-3.5 rounded-xl border p-3 text-left outline-none focus-visible:ring-3 focus-visible:ring-ring ${selected === i ? 'border-line bg-bubble' : 'border-transparent hover:bg-card'}`}
            >
              <Creature role={role.name} face={role.face} size={40} animate />
              <span className="min-w-0 flex-1 basis-40">
                <span className="flex items-center gap-2">
                  <span>{role.name || 'role'}</span>
                  {role.lead && (
                    <span className="rounded-pill bg-lead px-2 text-[11px] leading-5 text-mark-ink">
                      answers by default
                    </span>
                  )}
                </span>
                <span className="block truncate text-[12.5px] text-muted">{role.persona}</span>
              </span>
              <span className={`${badge} border-line text-soft`}>{backendLabel(role.backend)}</span>
              <span
                className={`${badge} ${role.write ? 'border-transparent bg-developer/15 text-developer' : 'border-line text-muted'}`}
              >
                {role.write ? 'edits code' : 'read-only'}
              </span>
              {role.backend === 'claude' && (
                <span className={`${badge} border-line text-muted`}>{role.tools.length} tools</span>
              )}
            </button>
          ))}
          <AddRoleRow
            onAdd={(role) => {
              setRoles([...roles, role]);
              setKeys([...keys, Math.max(-1, ...keys) + 1]);
              setSelected(roles.length);
            }}
          />
        </section>

        <section aria-label="Gates">
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
        </section>
      </div>

      {current && selected !== null && (
        <RolePanel
          key={keys[selected]}
          role={current}
          skills={skills}
          noProject={noProject}
          onChange={(r) => updateRole(selected, r)}
          onRemove={() => removeRole(selected)}
          onClose={() => setSelected(null)}
        />
      )}
    </>
  );
}
