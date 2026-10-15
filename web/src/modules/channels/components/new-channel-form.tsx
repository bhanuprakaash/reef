import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useCreateChannel } from '@/modules/channels/api/create-channel';
import { useTeams } from '@/lib/api/get-teams';
import { BACKENDS, PERMISSION_MODES } from '@/modules/channels/utils/backends';
import { Button } from '@/components/button';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';

type Props = { projectId: number; featureId: number | null };

export function NewChannelForm({ projectId, featureId }: Props) {
  const label = featureId === null ? 'New quick session' : 'New channel';
  const [open, setOpen] = useState(false);
  const [backend, setBackend] = useState('');
  const [permissionMode, setPermissionMode] = useState('');
  const [team, setTeam] = useState('solo');
  const [name, setName] = useState('');
  const create = useCreateChannel();
  const teams = useTeams();
  const navigate = useNavigate();

  if (!open) {
    return (
      <Button type="button" onClick={() => setOpen(true)}>
        {label}
      </Button>
    );
  }

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault();
        const time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        create.mutate(
          {
            projectId,
            featureId,
            name: name.trim() || `${backend || team} ${time}`,
            team,
            backend,
            permissionMode,
          },
          {
            onSuccess: (channel) => {
              setName('');
              setOpen(false);
              void navigate(`/channels/${channel.id}`);
            },
          },
        );
      }}
    >
      <label className="block space-y-1">
        <span className="text-sm text-muted">Backend</span>
        <Select
          className="w-full"
          aria-label="Backend"
          value={backend}
          onChange={(e) => {
            setBackend(e.target.value);
            setPermissionMode('');
          }}
        >
          <option value="">Team default</option>
          {BACKENDS.map((b) => (
            <option key={b} value={b}>
              {b}
            </option>
          ))}
        </Select>
      </label>
      {backend && (
        <label className="block space-y-1">
          <span className="text-sm text-muted">Permission mode</span>
          <Select
            className="w-full"
            aria-label="Permission mode"
            value={permissionMode}
            onChange={(e) => setPermissionMode(e.target.value)}
          >
            <option value="">Backend default</option>
            {PERMISSION_MODES[backend].map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </Select>
        </label>
      )}
      <label className="block space-y-1">
        <span className="text-sm text-muted">Team</span>
        <Select
          className="w-full"
          aria-label="Team"
          value={team}
          onChange={(e) => setTeam(e.target.value)}
        >
          {(teams.data ?? [{ name: 'solo' }]).map((t) => (
            <option key={t.name} value={t.name}>
              {t.name}
            </option>
          ))}
        </Select>
      </label>
      <TextField
        className="w-full"
        placeholder="Name (optional)"
        aria-label="Channel name"
        value={name}
        onChange={(e) => setName(e.target.value)}
      />
      {create.isError && <p className="text-sm text-danger">{create.error.message}</p>}
      <div className="flex gap-2">
        <Button variant="primary" type="submit" loading={create.isPending}>
          Start
        </Button>
        <Button
          variant="ghost"
          type="button"
          onClick={() => {
            create.reset();
            setOpen(false);
          }}
        >
          Cancel
        </Button>
      </div>
    </form>
  );
}
