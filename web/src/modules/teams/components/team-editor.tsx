import { useState } from 'react';
import { useDeleteTeam } from '@/modules/teams/api/delete-team';
import { useUpdateTeam } from '@/modules/teams/api/update-team';
import { AddRoleRow } from '@/modules/teams/components/add-role-row';
import { RolePanel } from '@/modules/teams/components/role-panel';
import type { Provider, Role, Skills, Team } from '@/types/api';
import { backendLabel } from '@/modules/teams/utils/backend-label';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';
import { Dialog } from '@/components/dialog';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

const badge = 'inline-flex h-[30px] items-center rounded-pill border px-3 text-xs';

type Props = {
  team: Team;
  openRole?: string;
  skills?: Skills;
  providers?: Provider[];
  noProject?: boolean;
  onSaved: (name: string) => void;
  onDeleted: () => void;
  onDuplicate: () => void;
};

export function TeamEditor({
  team,
  openRole,
  skills,
  providers,
  noProject,
  onSaved,
  onDeleted,
  onDuplicate,
}: Props) {
  const [loaded, setLoaded] = useState(team);
  const [responsibility, setResponsibility] = useState(team.responsibility);
  const [roles, setRoles] = useState<Role[]>(team.roles);
  const [keys, setKeys] = useState(() => team.roles.map((_, i) => i));
  const [selected, setSelected] = useState<number | null>(() => {
    const i = team.roles.findIndex((r) => r.name === openRole);
    return i < 0 ? null : i;
  });
  const save = useUpdateTeam();
  const remove = useDeleteTeam();
  const [confirmDelete, setConfirmDelete] = useState(false);

  function load(t: Team) {
    setLoaded(t);
    setResponsibility(t.responsibility);
    setRoles(t.roles);
    setKeys(t.roles.map((_, i) => i));
  }
  const unsaved =
    responsibility !== loaded.responsibility ||
    JSON.stringify(roles) !== JSON.stringify(loaded.roles);
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
            {team.name !== 'solo' && (
              <Button
                type="button"
                variant="ghost"
                className="text-danger!"
                onClick={() => setConfirmDelete(true)}
              >
                Delete
              </Button>
            )}
            <Button type="button" onClick={onDuplicate}>
              Duplicate
            </Button>
            <Button
              variant="primary"
              type="button"
              disabled={save.isPending}
              onClick={() =>
                save.mutate(
                  {
                    name: team.name,
                    team: { name: team.name, responsibility: responsibility.trim(), roles },
                  },
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

        <label className="block">
          <span className="mb-1.5 block text-xs text-muted">Responsibility</span>
          <TextArea
            className="min-h-24 w-full resize-y"
            value={responsibility}
            placeholder="What work should this team own?"
            onChange={(e) => setResponsibility(e.target.value)}
          />
        </label>

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
      </div>

      {current && selected !== null && (
        <RolePanel
          key={keys[selected]}
          role={current}
          skills={skills}
          providers={providers}
          noProject={noProject}
          onChange={(r) => updateRole(selected, r)}
          onRemove={() => removeRole(selected)}
          onClose={() => setSelected(null)}
        />
      )}
      <Dialog
        open={confirmDelete}
        onClose={() => setConfirmDelete(false)}
        title={`Delete ${team.name}?`}
      >
        <p className="text-sm text-muted">Its existing Spaces keep their copied teammates.</p>
        {remove.isError && <p className="mt-3 text-sm text-danger">{remove.error.message}</p>}
        <div className="mt-5 flex justify-end gap-2">
          <Button type="button" onClick={() => setConfirmDelete(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            loading={remove.isPending}
            onClick={() =>
              remove.mutate(team.name, {
                onSuccess: () => {
                  setConfirmDelete(false);
                  onDeleted();
                },
              })
            }
          >
            Delete team
          </Button>
        </div>
      </Dialog>
    </>
  );
}
