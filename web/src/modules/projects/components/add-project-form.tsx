import { useState } from 'react';
import { useCreateProject } from '@/modules/projects/api/create-project';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';
import type { Project } from '@/types/api';

type Props = { onClose: () => void; onCreated?: (project: Project) => void };

export function AddProjectForm({ onClose, onCreated }: Props) {
  const [name, setName] = useState('');
  const [path, setPath] = useState('');
  const create = useCreateProject();

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { name, path },
          {
            onSuccess: (project) => {
              onClose();
              onCreated?.(project);
            },
          },
        );
      }}
    >
      <div className="space-y-1.5">
        <label htmlFor="add-project-path" className="block text-[12.5px] text-muted">
          Repo folder
        </label>
        <div className="flex gap-2">
          <TextField
            id="add-project-path"
            className="min-w-0 flex-1 font-mono"
            placeholder="/Users/you/code/project"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            required
          />
          {window.reef && (
            <Button
              type="button"
              onClick={() => void window.reef?.chooseFolder().then((p) => p && setPath(p))}
            >
              Choose…
            </Button>
          )}
        </div>
      </div>
      <div className="space-y-1.5">
        <label htmlFor="add-project-name" className="block text-[12.5px] text-muted">
          Name
        </label>
        <TextField
          id="add-project-name"
          className="w-full"
          placeholder="project"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
      </div>
      {create.isError && <p className="text-sm text-danger">{create.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" loading={create.isPending}>
          Add project
        </Button>
      </div>
    </form>
  );
}
