import { useId, useState } from 'react';
import { useNavigate } from 'react-router';
import { useCreateSpace } from '@/modules/spaces/api/create-space';
import { useBots } from '@/lib/api/get-bots';
import { useTeams } from '@/lib/api/get-teams';
import type { Role, SpaceProposal } from '@/types/api';
import { Button } from '@/components/button';
import { Creature, GroupAvatar } from '@/components/creature';
import { TextField } from '@/components/text-field';

const labelClass = 'mb-1.5 block text-[12.5px] text-muted';

type Props = {
  projectId?: number;
  workItemId: number | null;
  itemTeam?: string;
  proposal?: SpaceProposal;
  onCreated?: () => void;
  onClose: () => void;
};

export function NewSpaceForm({
  projectId,
  workItemId,
  itemTeam,
  proposal,
  onCreated,
  onClose,
}: Props) {
  const [picked, setPicked] = useState(proposal ? [] : [itemTeam ?? 'solo']);
  const [dropped, setDropped] = useState<number[]>([]);
  const [chosen, setChosen] = useState<number | null>(proposal?.coordinatorBotId ?? null);
  const [name, setName] = useState(proposal?.name ?? '');
  const create = useCreateSpace();
  const teams = useTeams();
  const bots = useBots();
  const navigate = useNavigate();
  const nameId = useId();
  const teamList = (teams.data ?? []).filter((t) => itemTeam === undefined || t.name === itemTeam);

  const proposed: Role[] = (bots.data ?? [])
    .filter((b) => proposal?.botIds.includes(b.id))
    .map((b) => ({ ...b, botId: b.id, lead: false }));
  const pickedRoles = proposed.concat(
    picked.flatMap((n) => teams.data?.find((t) => t.name === n)?.roles ?? []),
  );
  const listed = pickedRoles.filter(
    (r, i) => pickedRoles.findIndex((o) => o.botId === r.botId) === i,
  );
  const roster = listed.filter((r) => !dropped.includes(r.botId));
  const coordinator =
    roster.find((r) => r.botId === chosen) ?? roster.find((r) => r.lead) ?? roster[0];

  function toggleCrew(crew: string) {
    setPicked(picked.includes(crew) ? picked.filter((n) => n !== crew) : [...picked, crew]);
  }
  function toggleBot(r: Role) {
    setDropped(
      dropped.includes(r.botId) ? dropped.filter((b) => b !== r.botId) : [...dropped, r.botId],
    );
  }

  return (
    <form
      className="space-y-3.5"
      onSubmit={(e) => {
        e.preventDefault();
        if (!coordinator) return;
        const time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        create.mutate(
          {
            projectId,
            workItemId,
            name: name.trim() || `${picked[0] ?? 'space'} ${time}`,
            team: picked[0],
            botIds: roster.map((r) => r.botId),
            coordinatorBotId: coordinator.botId,
          },
          {
            onSuccess: (space) => {
              onCreated?.();
              onClose();
              void navigate(`/spaces/${space.id}`);
            },
          },
        );
      }}
    >
      <div>
        <label htmlFor={nameId} className={labelClass}>
          Name
        </label>
        <TextField
          id={nameId}
          className="w-full"
          placeholder="optional"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      <div>
        <div className={labelClass}>Crews</div>
        <div className="grid grid-cols-2 gap-2">
          {teamList.map((t) => (
            <button
              key={t.name}
              type="button"
              aria-pressed={picked.includes(t.name)}
              onClick={() => toggleCrew(t.name)}
              className={`flex min-w-0 items-center gap-2 rounded-menu border p-2.5 text-left text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring ${
                picked.includes(t.name) ? 'border-fg bg-raised text-fg' : 'border-line text-soft'
              }`}
            >
              <GroupAvatar
                roles={t.roles.map((r) => r.botId)}
                faces={Object.fromEntries(t.roles.map((r) => [r.botId, r]))}
                size={26}
              />
              <span className="truncate">{t.name}</span>
            </button>
          ))}
        </div>
      </div>
      <div>
        <div className={labelClass}>Bots in this Space</div>
        <ul className="space-y-1">
          {listed.map((r) => {
            const out = dropped.includes(r.botId);
            return (
              <li key={r.botId} className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  aria-label={`Include ${r.name}`}
                  checked={!out}
                  onChange={() => toggleBot(r)}
                />
                <Creature look={r} size={22} />
                <span className={`min-w-0 flex-1 truncate ${out ? 'text-faint' : ''}`}>
                  {r.name}
                </span>
                {!out && (
                  <label className="flex items-center gap-1.5 text-xs text-muted">
                    <input
                      type="radio"
                      name="coordinator"
                      aria-label={`${r.name} coordinates`}
                      checked={coordinator?.botId === r.botId}
                      onChange={() => setChosen(r.botId)}
                    />
                    coordinator
                  </label>
                )}
              </li>
            );
          })}
        </ul>
        <p className="mt-1.5 text-xs text-muted">
          A Crew only lists Bots. The coordinator gets messages that mention nobody.
        </p>
      </div>
      {create.isError && <p className="text-sm text-danger">{create.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          onClick={() => {
            create.reset();
            onClose();
          }}
        >
          Cancel
        </Button>
        <Button variant="primary" type="submit" disabled={!coordinator} loading={create.isPending}>
          Start
        </Button>
      </div>
    </form>
  );
}
