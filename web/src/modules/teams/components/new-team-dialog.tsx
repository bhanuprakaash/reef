import { useState } from 'react';
import { useUpdateTeam } from '@/modules/teams/api/update-team';
import type { Team } from '@/types/api';
import { Button } from '@/components/button';
import { Dialog } from '@/components/dialog';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';

const teamNamePattern = /^[A-Za-z0-9_-]+$/;

type Props = {
  open: boolean;
  teams: Team[];
  onClose: () => void;
  onCreated: (name: string) => void;
};

function NewTeamForm({ teams, onClose, onCreated }: Omit<Props, 'open'>) {
  const [name, setName] = useState('');
  const [from, setFrom] = useState(() => (teams.find((t) => t.name === 'solo') ?? teams[0]).name);
  const create = useUpdateTeam();
  const source = teams.find((t) => t.name === from);

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (!source) return;
        const team = { name: name.trim(), roles: source.roles, gates: source.gates };
        create.mutate(
          { name: team.name, team, from: source.name },
          { onSuccess: (saved) => onCreated(saved.name) },
        );
      }}
    >
      <label className="block">
        <span className="mb-1.5 block text-[12.5px] text-muted">Name</span>
        <TextField
          className="w-full"
          autoFocus
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </label>
      <label className="block">
        <span className="mb-1.5 block text-[12.5px] text-muted">Start from</span>
        <Select
          className="w-full"
          aria-label="Start from"
          value={from}
          onChange={setFrom}
          options={teams.map((t) => ({ value: t.name, label: t.name }))}
        />
      </label>
      <p className="text-[12.5px] text-muted">
        Copies the roles and gates of {from}. Edit them after.
      </p>
      {create.isError && <p className="text-sm text-danger">{create.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button
          variant="primary"
          type="submit"
          disabled={!teamNamePattern.test(name.trim())}
          loading={create.isPending}
        >
          Create team
        </Button>
      </div>
    </form>
  );
}

export function NewTeamDialog({ open, ...rest }: Props) {
  return (
    <Dialog open={open} onClose={rest.onClose} title="New team">
      {open && <NewTeamForm {...rest} />}
    </Dialog>
  );
}
