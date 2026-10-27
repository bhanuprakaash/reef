import { useState } from 'react';
import { useCreateTeammate } from '@/modules/teams/api/create-teammate';
import type { Role } from '@/types/api';
import { CreaturePicker } from '@/modules/teams/components/creature-picker';
import { backendOptions } from '@/modules/teams/utils/backend-label';
import { suggestFace } from '@/modules/teams/utils/suggest-face';
import { Button } from '@/components/button';
import { Dialog } from '@/components/dialog';
import { Select } from '@/components/select';
import { TextField } from '@/components/text-field';

const rolePattern = /^[A-Za-z0-9_-]+$/;

const blankRole: Role = {
  name: '',
  lead: false,
  write: false,
  backend: 'claude',
  permissionMode: '',
  persona: '',
  skills: [],
  tools: [],
  mcp: [],
};

type FormProps = { onCreated: (id: number) => void; onClose: () => void };

function NewTeammateForm({ onCreated, onClose }: FormProps) {
  const [name, setName] = useState('');
  const [picked, setPicked] = useState('');
  const [backend, setBackend] = useState('claude');
  const face = picked || suggestFace(name);
  const valid = rolePattern.test(name.trim());
  const create = useCreateTeammate();

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { ...blankRole, name: name.trim(), backend, face },
          {
            onSuccess: (saved) => {
              onCreated(saved.id);
              onClose();
            },
          },
        );
      }}
    >
      <div className="flex justify-center py-1">
        <CreaturePicker value={face} onChange={setPicked} size={56} />
      </div>
      <label className="block">
        <span className="mb-1.5 block text-[12.5px] text-muted">Teammate name</span>
        <TextField
          className="w-full"
          autoFocus
          aria-invalid={name.trim() !== '' && !valid}
          placeholder="e.g. researcher"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </label>
      <label className="block">
        <span className="mb-1.5 block text-[12.5px] text-muted">Backend</span>
        <Select
          className="w-full"
          aria-label="Teammate backend"
          value={backend}
          onChange={setBackend}
          options={backendOptions}
        />
      </label>
      {create.isError && <p className="text-sm text-danger">{create.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" disabled={!valid} loading={create.isPending}>
          Create teammate
        </Button>
      </div>
    </form>
  );
}

type Props = { open: boolean; onClose: () => void; onCreated: (id: number) => void };

export function NewTeammateDialog({ open, onClose, onCreated }: Props) {
  return (
    <Dialog open={open} onClose={onClose} title="New teammate">
      {open && <NewTeammateForm onCreated={onCreated} onClose={onClose} />}
    </Dialog>
  );
}
