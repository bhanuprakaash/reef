import { useState } from 'react';
import { useCreateProject } from '@/modules/projects/api/create-project';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';
import type { Project } from '@/types/api';

export function AddProjectForm({ onCreated }: { onCreated?: (project: Project) => void }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [path, setPath] = useState('');
  const create = useCreateProject();

  if (!open) {
    return (
      <Button type="button" onClick={() => setOpen(true)}>
        Add project
      </Button>
    );
  }

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { name, path },
          {
            onSuccess: (project) => {
              setName('');
              setPath('');
              setOpen(false);
              onCreated?.(project);
            },
          },
        );
      }}
    >
      <TextField
        className="w-full"
        placeholder="Name"
        aria-label="Project name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />
      <TextField
        className="w-full"
        placeholder="/absolute/path/to/repo"
        aria-label="Project path"
        value={path}
        onChange={(e) => setPath(e.target.value)}
        required
      />
      {create.isError && <p className="text-sm text-danger">{create.error.message}</p>}
      <div className="flex gap-2">
        <Button variant="primary" type="submit" loading={create.isPending}>
          Add
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
