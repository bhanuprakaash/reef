import { useId, useState } from 'react';
import { useNavigate } from 'react-router';
import { useCreateChannel } from '@/modules/channels/api/create-channel';
import { useTeams } from '@/lib/api/get-teams';
import { BACKENDS, PERMISSION_MODES } from '@/modules/channels/utils/backends';
import { Button } from '@/components/button';
import { GroupAvatar } from '@/components/creature';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';

const labelClass = 'mb-1.5 block text-[12.5px] text-muted';

type Props = { projectId: number; featureId: number | null; onClose: () => void };

export function NewChannelForm({ projectId, featureId, onClose }: Props) {
  const [backend, setBackend] = useState('');
  const [permissionMode, setPermissionMode] = useState('');
  const [team, setTeam] = useState('solo');
  const [name, setName] = useState('');
  const create = useCreateChannel();
  const teams = useTeams();
  const navigate = useNavigate();
  const nameId = useId();
  const teamList = teams.data ?? [{ name: 'solo', roles: [{ name: 'lead', face: undefined }] }];

  return (
    <form
      className="space-y-3.5"
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
              onClose();
              void navigate(`/channels/${channel.id}`);
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
      <div className="grid grid-cols-2 gap-2">
        <div>
          <div className={labelClass}>Backend</div>
          <Select
            className="w-full"
            aria-label="Backend"
            value={backend}
            onChange={(v) => {
              setBackend(v);
              setPermissionMode('');
            }}
            options={[
              { value: '', label: 'Team default' },
              ...BACKENDS.map((b) => ({ value: b, label: b })),
            ]}
          />
        </div>
        <div>
          <div className={labelClass}>Permission</div>
          <Select
            className="w-full"
            aria-label="Permission mode"
            disabled={!backend}
            value={permissionMode}
            onChange={setPermissionMode}
            options={[
              { value: '', label: 'Backend default' },
              ...(PERMISSION_MODES[backend] ?? []).map((m) => ({ value: m, label: m })),
            ]}
          />
        </div>
      </div>
      <div>
        <div className={labelClass}>Team</div>
        <div className="grid grid-cols-2 gap-2">
          {teamList.map((t) => (
            <button
              key={t.name}
              type="button"
              aria-pressed={t.name === team}
              onClick={() => setTeam(t.name)}
              className={`flex min-w-0 items-center gap-2 rounded-menu border p-2.5 text-left text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring ${
                t.name === team ? 'border-fg bg-raised text-fg' : 'border-line text-soft'
              }`}
            >
              <GroupAvatar
                roles={t.roles.map((r) => r.name)}
                faces={Object.fromEntries(t.roles.map((r) => [r.name, r.face]))}
                size={26}
              />
              <span className="truncate">{t.name}</span>
            </button>
          ))}
        </div>
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
        <Button variant="primary" type="submit" loading={create.isPending}>
          Start
        </Button>
      </div>
    </form>
  );
}
