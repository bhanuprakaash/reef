import { useState } from 'react';
import { useCreateProject } from '@/modules/projects/api/create-project';
import { buttonClass, errorClass, inputClass } from '@/utils/classes';

export function AddProjectForm() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [path, setPath] = useState('');
  const create = useCreateProject();

  if (!open) {
    return (
      <button type="button" className={buttonClass} onClick={() => setOpen(true)}>
        Add project
      </button>
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
            onSuccess: () => {
              setName('');
              setPath('');
              setOpen(false);
            },
          },
        );
      }}
    >
      <input
        className={`${inputClass} w-full`}
        placeholder="Name"
        aria-label="Project name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />
      <input
        className={`${inputClass} w-full`}
        placeholder="/absolute/path/to/repo"
        aria-label="Project path"
        value={path}
        onChange={(e) => setPath(e.target.value)}
        required
      />
      {create.isError && <p className={errorClass}>{create.error.message}</p>}
      <div className="flex gap-2">
        <button type="submit" className={buttonClass} disabled={create.isPending}>
          Add
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
