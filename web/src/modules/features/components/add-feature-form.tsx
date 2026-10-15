import { useState } from 'react';
import { useCreateFeature } from '@/modules/features/api/create-feature';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';

type Props = { projectId: number; onClose?: () => void };

export function AddFeatureForm({ projectId, onClose }: Props) {
  const [open, setOpen] = useState(onClose !== undefined);
  const [slug, setSlug] = useState('');
  const [name, setName] = useState('');
  const create = useCreateFeature();

  function close() {
    setOpen(false);
    onClose?.();
  }

  if (!open) {
    return (
      <Button type="button" onClick={() => setOpen(true)}>
        Add feature
      </Button>
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
              close();
            },
          },
        );
      }}
    >
      <TextField
        className="w-full"
        placeholder="slug, e.g. login"
        aria-label="Feature slug"
        value={slug}
        onChange={(e) => setSlug(e.target.value)}
        required
      />
      <TextField
        className="w-full"
        placeholder="Name"
        aria-label="Feature name"
        value={name}
        onChange={(e) => setName(e.target.value)}
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
            close();
          }}
        >
          Cancel
        </Button>
      </div>
    </form>
  );
}
