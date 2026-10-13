import { useState } from 'react';
import { useCreateFeature } from '@/modules/features/api/create-feature';
import { buttonClass, errorClass, inputClass } from '@/utils/classes';

export function AddFeatureForm({ projectId }: { projectId: number }) {
  const [open, setOpen] = useState(false);
  const [slug, setSlug] = useState('');
  const [name, setName] = useState('');
  const create = useCreateFeature();

  if (!open) {
    return (
      <button type="button" className={buttonClass} onClick={() => setOpen(true)}>
        Add feature
      </button>
    );
  }

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { projectId, slug, name },
          {
            onSuccess: () => {
              setSlug('');
              setName('');
              setOpen(false);
            },
          },
        );
      }}
    >
      <input
        className={`${inputClass} w-full`}
        placeholder="slug, e.g. login"
        aria-label="Feature slug"
        value={slug}
        onChange={(e) => setSlug(e.target.value)}
        required
      />
      <input
        className={`${inputClass} w-full`}
        placeholder="Name"
        aria-label="Feature name"
        value={name}
        onChange={(e) => setName(e.target.value)}
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
