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
  proposal?: SpaceProposal;
  onCreated?: () => void;
  onClose: () => void;
};

export function NewSpaceForm({ projectId, workItemId, proposal, onCreated, onClose }: Props) {
  const [picked, setPicked] = useState<string[]>([]);
  const [included, setIncluded] = useState<number[]>(proposal?.botIds ?? []);
  const [chosen, setChosen] = useState<number | null>(proposal?.coordinatorBotId ?? null);
  const [name, setName] = useState(proposal?.name ?? '');
  const create = useCreateSpace();
  const teams = useTeams();
  const bots = useBots();
  const navigate = useNavigate();
  const nameId = useId();
  const teamList = teams.data ?? [];

  const crewBots = (crew: string) =>
    teams.data?.find((t) => t.name === crew)?.roles.map((r) => r.botId) ?? [];
  const roster: Role[] = (bots.data ?? [])
    .filter((b) => included.includes(b.id))
    .map((b) => ({ ...b, botId: b.id, lead: false }));
  const suggested = picked.flatMap(
    (n) => teams.data?.find((t) => t.name === n)?.roles.filter((r) => r.lead) ?? [],
  );
  const coordinator =
    roster.find((r) => r.botId === chosen) ??
    roster.find((r) => suggested.some((s) => s.botId === r.botId)) ??
    roster[0];
  const needsCoordinator = workItemId === null;
  const canStart =
    (!needsCoordinator || coordinator !== undefined) && (workItemId === null || name.trim() !== '');

  function toggleCrew(crew: string) {
    const next = picked.includes(crew) ? picked.filter((n) => n !== crew) : [...picked, crew];
    setPicked(next);
    setIncluded(
      next.includes(crew)
        ? [...new Set([...included, ...crewBots(crew)])]
        : included.filter(
            (id) => !crewBots(crew).includes(id) || next.some((n) => crewBots(n).includes(id)),
          ),
    );
  }
  function toggleBot(id: number) {
    setIncluded(included.includes(id) ? included.filter((b) => b !== id) : [...included, id]);
  }

  return (
    <form
      className="space-y-3.5"
      onSubmit={(e) => {
        e.preventDefault();
        if (!canStart) return;
        const time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        create.mutate(
          {
            projectId,
            workItemId,
            name: name.trim() || `${picked[0] ?? 'space'} ${time}`,
            team: needsCoordinator ? picked[0] : undefined,
            botIds: needsCoordinator ? roster.map((r) => r.botId) : undefined,
            coordinatorBotId: needsCoordinator ? coordinator?.botId : undefined,
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
          placeholder={workItemId === null ? 'optional' : 'required'}
          required={workItemId !== null}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      {workItemId !== null ? (
        <p className="text-xs text-muted">
          This Space uses the Work item’s owner and participants.
        </p>
      ) : (
        <>
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
                    picked.includes(t.name)
                      ? 'border-fg bg-raised text-fg'
                      : 'border-line text-soft'
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
              {(bots.data ?? []).map((b) => {
                const out = !included.includes(b.id);
                return (
                  <li key={b.id} className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      aria-label={`Include ${b.name}`}
                      checked={!out}
                      onChange={() => toggleBot(b.id)}
                    />
                    <Creature look={b} size={22} />
                    <span className={`min-w-0 flex-1 truncate ${out ? 'text-faint' : ''}`}>
                      {b.name}
                    </span>
                    {!out && (
                      <label className="flex items-center gap-1.5 text-xs text-muted">
                        <input
                          type="radio"
                          name="coordinator"
                          aria-label={`${b.name} coordinates`}
                          checked={coordinator?.botId === b.id}
                          onChange={() => setChosen(b.id)}
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
        </>
      )}
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
        <Button variant="primary" type="submit" disabled={!canStart} loading={create.isPending}>
          Start
        </Button>
      </div>
    </form>
  );
}
