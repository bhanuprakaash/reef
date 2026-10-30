import { useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { useDeleteTeam } from '@/modules/teams/api/delete-team';
import { useUpdateTeam } from '@/modules/teams/api/update-team';
import { AddMemberRow } from '@/modules/teams/components/add-member-row';
import { RoleRow } from '@/modules/teams/components/role-row';
import type { Role, Team } from '@/types/api';
import { Button, buttonClass } from '@/components/button';
import { Dialog } from '@/components/dialog';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';
import { Toggle } from '@/components/toggle';

type Props = {
  team: Team;
  onSaved: (name: string) => void;
  onDeleted: () => void;
  onDuplicate: () => void;
};

export function TeamEditor({ team, onSaved, onDeleted, onDuplicate }: Props) {
  const [loaded, setLoaded] = useState(team);
  const [responsibility, setResponsibility] = useState(team.responsibility);
  const [roles, setRoles] = useState<Role[]>(team.roles);
  const navigate = useNavigate();
  const save = useUpdateTeam();
  const remove = useDeleteTeam();
  const [confirmDelete, setConfirmDelete] = useState(false);

  function load(t: Team) {
    setLoaded(t);
    setResponsibility(t.responsibility);
    setRoles(t.roles);
  }
  const unsaved =
    responsibility !== loaded.responsibility ||
    JSON.stringify(roles) !== JSON.stringify(loaded.roles);
  if (team !== loaded && !unsaved) load(team);

  function updateRole(i: number, role: Role) {
    setRoles(roles.map((r, idx) => (idx === i ? role : r)));
  }
  function removeRole(i: number) {
    setRoles(roles.filter((_, idx) => idx !== i));
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
            <Link to={`/teams/${encodeURIComponent(team.name)}`} className={buttonClass()}>
              Queue
            </Link>
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
            <div key={role.botId} className="flex items-center gap-2">
              <div className="min-w-0 flex-1">
                <RoleRow
                  role={role}
                  selected={false}
                  onSelect={() => void navigate(`/bots?bot=${role.botId}`)}
                />
              </div>
              <Toggle
                label={`${role.name} lead`}
                checked={role.lead}
                onChange={(lead) => updateRole(i, { ...role, lead })}
              />
              <Button type="button" variant="ghost" onClick={() => removeRole(i)}>
                Remove
              </Button>
            </div>
          ))}
          <AddMemberRow
            team={team.name}
            members={roles}
            onAdded={(role) => {
              setRoles([...roles, role]);
              setLoaded({ ...loaded, roles: [...loaded.roles, role] });
            }}
          />
        </section>
      </div>

      <Dialog
        open={confirmDelete}
        onClose={() => setConfirmDelete(false)}
        title={`Delete ${team.name}?`}
      >
        <p className="text-sm text-muted">Its existing Spaces keep their Bots.</p>
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
