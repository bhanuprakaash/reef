import { useRef, useState } from 'react';
import { Link } from 'react-router';
import { useBots } from '@/lib/api/get-bots';
import { useDismiss } from '@/lib/use-dismiss';
import { useDeleteTeam } from '@/modules/teams/api/delete-team';
import { useUpdateTeam } from '@/modules/teams/api/update-team';
import type { Role, Team } from '@/types/api';
import { Button } from '@/components/button';
import { Creature } from '@/components/creature';
import { Dialog } from '@/components/dialog';
import { Menu, MenuItem } from '@/components/menu';
import { SearchPicker } from '@/components/search-picker';

type Props = { team: Team; onDeleted: () => void; onDuplicate: () => void };

const ghostField =
  'w-full rounded-field border border-transparent bg-transparent px-2 -mx-2 outline-none hover:border-line focus-visible:border-fg focus-visible:ring-3 focus-visible:ring-ring';

export function TeamEditor({ team, onDeleted, onDuplicate }: Props) {
  const bots = useBots();
  const save = useUpdateTeam();
  const remove = useDeleteTeam();
  const [description, setDescription] = useState(team.responsibility);
  const [menu, setMenu] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  useDismiss(menu, menuRef, () => setMenu(false));

  const shown = save.isPending ? save.variables.team : team;
  const roles = shown.roles;
  function put(next: { responsibility?: string; roles?: Role[] }) {
    save.mutate({
      name: team.name,
      team: { name: team.name, responsibility: shown.responsibility, roles, ...next },
    });
  }
  function toggle(id: number) {
    const bot = bots.data?.find((b) => b.id === id);
    if (roles.some((r) => r.botId === id)) put({ roles: roles.filter((r) => r.botId !== id) });
    else if (bot) put({ roles: [...roles, { ...bot, botId: id, lead: false }] });
  }

  return (
    <div className="min-w-0 flex-1">
      <div className="flex items-center gap-2">
        <h2 className="min-w-0 flex-1 truncate text-2xl tracking-[-0.015em]">{team.name}</h2>
        {save.isSuccess && (
          <span className="flex items-center gap-1.5 text-[11.5px] text-done">
            <svg
              className="size-3"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.4"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M5 12l5 5 9-10" />
            </svg>
            Saved
          </span>
        )}
        <div ref={menuRef} className="relative">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label="Crew actions"
            aria-haspopup="menu"
            aria-expanded={menu}
            onClick={() => setMenu(!menu)}
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <circle cx="5" cy="12" r="1.8" />
              <circle cx="12" cy="12" r="1.8" />
              <circle cx="19" cy="12" r="1.8" />
            </svg>
          </Button>
          {menu && (
            <Menu className="absolute top-full right-0 z-30 mt-1.5 w-48">
              <MenuItem
                onClick={() => {
                  setMenu(false);
                  onDuplicate();
                }}
              >
                Duplicate
              </MenuItem>
              {team.name !== 'solo' && (
                <MenuItem
                  destructive
                  onClick={() => {
                    setMenu(false);
                    setConfirmDelete(true);
                  }}
                >
                  Delete crew…
                </MenuItem>
              )}
            </Menu>
          )}
        </div>
      </div>
      <input
        aria-label="Description"
        className={`${ghostField} mt-1 h-11 text-[13.5px] text-soft placeholder:text-faint md:h-8`}
        value={description}
        placeholder="Add a description (optional)"
        onChange={(e) => setDescription(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
        onBlur={() => {
          const next = description.trim();
          setDescription(next);
          if (next !== team.responsibility) put({ responsibility: next });
        }}
      />
      {save.isError && <p className="mt-2 text-sm text-danger">{save.error.message}</p>}

      <section aria-label="Bots" className="mt-6">
        <div className="mb-1.5 flex flex-wrap items-center gap-x-4 text-[13px] text-muted">
          <h3 className="font-normal">Bots · {roles.length}</h3>
          <span className="text-xs text-faint md:ml-auto">
            The starred Bot answers when no one is @mentioned
          </span>
        </div>
        <ul className="border-t border-hairline">
          {roles.map((role) => (
            <li
              key={role.botId}
              className="group flex min-h-14 items-center gap-3 border-b border-hairline px-2 py-1.5"
            >
              <Link
                to={`/bots?bot=${role.botId}`}
                className="flex min-w-0 flex-1 items-center gap-3 rounded-lg outline-none focus-visible:ring-3 focus-visible:ring-ring"
              >
                <Creature look={role} size={34} animate />
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{role.name}</span>
                  <span className="block truncate text-[12.5px] text-muted">
                    {role.description}
                  </span>
                </span>
              </Link>
              <button
                type="button"
                aria-pressed={role.lead}
                aria-label={`${role.name} answers first`}
                title="Answers first when you don't @mention anyone"
                onClick={() =>
                  put({
                    roles: roles.map((r) => ({ ...r, lead: r.botId === role.botId && !r.lead })),
                  })
                }
                className={`flex h-11 items-center gap-1.5 rounded-pill px-2 text-xs outline-none hover:bg-hover focus-visible:ring-3 focus-visible:ring-ring md:h-[30px] ${role.lead ? 'text-lead' : 'text-faint hover:text-soft'}`}
              >
                {role.lead ? (
                  <span>Answers first</span>
                ) : (
                  <span className="hidden group-hover:inline">Make default</span>
                )}
                <svg
                  className="size-4"
                  viewBox="0 0 24 24"
                  fill={role.lead ? 'currentColor' : 'none'}
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <path d="m12 3 2.7 5.7 6.2.8-4.6 4.3 1.2 6.2L12 17l-5.5 3 1.2-6.2L3.1 9.5l6.2-.8z" />
                </svg>
              </button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`Remove ${role.name}`}
                onClick={() => toggle(role.botId)}
              >
                <svg
                  className="size-4"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinecap="round"
                  aria-hidden="true"
                >
                  <path d="M6 6l12 12M18 6 6 18" />
                </svg>
              </Button>
            </li>
          ))}
        </ul>
        <div className="mt-2.5">
          <SearchPicker
            row
            label="Add Bot"
            options={(bots.data ?? []).map((b) => ({
              value: String(b.id),
              label: b.name,
              hint: b.description,
            }))}
            selected={roles.map((r) => String(r.botId))}
            onToggle={(id) => toggle(Number(id))}
          />
        </div>
      </section>

      <Dialog
        open={confirmDelete}
        onClose={() => setConfirmDelete(false)}
        title={`Delete ${team.name}?`}
      >
        <p className="text-sm text-muted">Its Bots and the members of existing Spaces stay.</p>
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
            Delete crew
          </Button>
        </div>
      </Dialog>
    </div>
  );
}
