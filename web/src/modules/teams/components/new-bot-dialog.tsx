import { useState } from 'react';
import { useCreateBot } from '@/modules/teams/api/create-bot';
import type { Role } from '@/types/api';
import { CreaturePicker } from '@/modules/teams/components/creature-picker';
import { backendOptions } from '@/modules/teams/utils/backend-label';
import { Button } from '@/components/button';
import type { Look } from '@/components/creature';
import { Dialog } from '@/components/dialog';
import { Select } from '@/components/select';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

const rolePattern = /^[A-Za-z0-9_-]+$/;

const blankRole: Omit<Role, 'botId'> = {
  name: '',
  lead: false,
  write: false,
  backend: 'claude',
  permissionMode: '',
  description: '',
  skills: [],
  tools: [],
  connections: [],
};

type FormProps = { onCreated: (id: number) => void; onClose: () => void };

function NewBotForm({ onCreated, onClose }: FormProps) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [look, setLook] = useState<Look>({ face: 'pebble' });
  const [backend, setBackend] = useState('claude');
  const valid = rolePattern.test(name.trim());
  const create = useCreateBot();

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { ...blankRole, name: name.trim(), description: description.trim(), backend, ...look },
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
        <CreaturePicker value={look} onChange={setLook} size={56} />
      </div>
      <label className="block">
        <span className="mb-1.5 block text-[12.5px] text-muted">Name</span>
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
        <span className="mb-1.5 block text-[12.5px] text-muted">Describe this Bot</span>
        <TextArea
          className="w-full"
          rows={4}
          placeholder="e.g. Reads the code and explains how it works."
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </label>
      <label className="block">
        <span className="mb-1.5 block text-[12.5px] text-muted">Backend</span>
        <Select
          className="w-full"
          aria-label="Bot backend"
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
          Create Bot
        </Button>
      </div>
    </form>
  );
}

type Props = { open: boolean; onClose: () => void; onCreated: (id: number) => void };

export function NewBotDialog({ open, onClose, onCreated }: Props) {
  return (
    <Dialog open={open} onClose={onClose} title="New Bot">
      {open && <NewBotForm onCreated={onCreated} onClose={onClose} />}
    </Dialog>
  );
}
