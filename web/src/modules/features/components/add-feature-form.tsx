import { useState } from 'react';
import { useCreateFeature } from '@/modules/features/api/create-feature';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';
import type { Feature } from '@/types/api';

type Props = { projectId: number; onClose: () => void; onCreated?: (feature: Feature) => void };

export function AddFeatureForm({ projectId, onClose, onCreated }: Props) {
  const [slug, setSlug] = useState('');
  const [name, setName] = useState('');
  const create = useCreateFeature();

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { projectId, slug, name },
          {
            onSuccess: (feature) => {
              onClose();
              onCreated?.(feature);
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
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" loading={create.isPending}>
          Add
        </Button>
      </div>
    </form>
  );
}
