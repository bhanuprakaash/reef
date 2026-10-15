import { useState } from 'react';
import type { Role } from '@/types/api';
import { CreaturePicker } from '@/modules/teams/components/creature-picker';
import { backendOptions } from '@/modules/teams/utils/backend-label';
import { Button } from '@/components/button';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';

const blankRole: Role = {
  name: '',
  lead: false,
  write: false,
  backend: 'claude',
  permissionMode: '',
  persona: '',
  skills: '',
  tools: [],
  mcp: [],
};

export function AddRoleCard({ onAdd }: { onAdd: (role: Role) => void }) {
  const [name, setName] = useState('');
  const [face, setFace] = useState('');
  const [backend, setBackend] = useState('claude');

  return (
    <div className="flex flex-col gap-3.5 rounded-2xl border border-dashed border-faint p-4">
      <div className="flex items-center justify-between">
        <span>Add a role</span>
        <span className="text-xs text-muted">Pick a creature</span>
      </div>
      <CreaturePicker value={face} onChange={setFace} />
      <div className="flex flex-wrap items-center gap-2.5">
        <TextField
          className="flex-1 basis-48"
          aria-label="Role name"
          placeholder="Role name, e.g. researcher"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <Select
          className="w-40"
          aria-label="Role backend"
          value={backend}
          onChange={setBackend}
          options={backendOptions}
        />
        <Button
          type="button"
          variant="primary"
          disabled={name.trim() === ''}
          onClick={() => {
            onAdd({ ...blankRole, name: name.trim(), backend, face: face || undefined });
            setName('');
            setFace('');
          }}
        >
          Add role
        </Button>
      </div>
    </div>
  );
}
