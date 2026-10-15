import { useState } from 'react';
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
  skills: '',
  tools: [],
  mcp: [],
};

type FormProps = { onAdd: (role: Role) => void; onClose: () => void };

function AddRoleForm({ onAdd, onClose }: FormProps) {
  const [name, setName] = useState('');
  const [picked, setPicked] = useState('');
  const [backend, setBackend] = useState('claude');
  const face = picked || suggestFace(name);
  const valid = rolePattern.test(name.trim());

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        onAdd({ ...blankRole, name: name.trim(), backend, face });
        onClose();
      }}
    >
      <div className="flex justify-center py-1">
        <CreaturePicker value={face} onChange={setPicked} size={56} />
      </div>
      <label className="block">
        <span className="mb-1.5 block text-[12.5px] text-muted">Role name</span>
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
          aria-label="Role backend"
          value={backend}
          onChange={setBackend}
          options={backendOptions}
        />
      </label>
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" disabled={!valid}>
          Add role
        </Button>
      </div>
    </form>
  );
}

export function AddRoleRow({ onAdd }: { onAdd: (role: Role) => void }) {
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex h-12 w-full items-center justify-center gap-2 rounded-2xl border border-dashed border-faint text-sm text-soft outline-none hover:bg-card focus-visible:ring-3 focus-visible:ring-ring"
      >
        <span aria-hidden="true">+</span> Add a role
      </button>
      <Dialog open={open} onClose={close} title="Add a role">
        {open && <AddRoleForm onAdd={onAdd} onClose={close} />}
      </Dialog>
    </>
  );
}
