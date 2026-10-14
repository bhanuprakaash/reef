import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useCreateChannel } from '@/modules/channels/api/create-channel';
import { useTeams } from '@/lib/api/get-teams';
import { BACKENDS, PERMISSION_MODES } from '@/modules/channels/utils/backends';
import { buttonClass, errorClass, inputClass, mutedClass } from '@/utils/classes';

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
      <button type="button" className={buttonClass} onClick={() => setOpen(true)}>
        {label}
      </button>
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
        <span className={mutedClass}>Backend</span>
        <select
          className={`${inputClass} w-full`}
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
        </select>
      </label>
      {backend && (
        <label className="block space-y-1">
          <span className={mutedClass}>Permission mode</span>
          <select
            className={`${inputClass} w-full`}
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
          </select>
        </label>
      )}
      <label className="block space-y-1">
        <span className={mutedClass}>Team</span>
        <select
          className={`${inputClass} w-full`}
          aria-label="Team"
          value={team}
          onChange={(e) => setTeam(e.target.value)}
        >
          {(teams.data ?? [{ name: 'solo' }]).map((t) => (
            <option key={t.name} value={t.name}>
              {t.name}
            </option>
          ))}
        </select>
      </label>
      <input
        className={`${inputClass} w-full`}
        placeholder="Name (optional)"
        aria-label="Channel name"
        value={name}
        onChange={(e) => setName(e.target.value)}
      />
      {create.isError && <p className={errorClass}>{create.error.message}</p>}
      <div className="flex gap-2">
        <button type="submit" className={buttonClass} disabled={create.isPending}>
          Start
        </button>
        <button
          type="button"
          className={buttonClass}
          onClick={() => {
            create.reset();
            setOpen(false);
          }}
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
