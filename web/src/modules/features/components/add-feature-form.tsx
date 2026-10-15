import { useId, useState } from 'react';
import { useCreateFeature } from '@/modules/features/api/create-feature';
import { Button } from '@/components/button';
import { TextField } from '@/components/text-field';
import { isValidSlug, slugify } from '@/modules/features/utils/slugify';
import type { Feature } from '@/types/api';

type Props = { projectId: number; onClose: () => void; onCreated?: (feature: Feature) => void };

export function AddFeatureForm({ projectId, onClose, onCreated }: Props) {
  const [slug, setSlug] = useState('');
  const [name, setName] = useState('');
  const [slugEdited, setSlugEdited] = useState(false);
  const hintId = useId();
  const slugInvalid = slug !== '' && !isValidSlug(slug);
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
        placeholder="Name"
        aria-label="Feature name"
        value={name}
        onChange={(e) => {
          setName(e.target.value);
          if (!slugEdited) setSlug(slugify(e.target.value));
        }}
        required
      />
      <div className="space-y-1">
        <TextField
          className="w-full"
          placeholder="slug, e.g. login"
          aria-label="Feature slug"
          aria-describedby={hintId}
          aria-invalid={slugInvalid || undefined}
          value={slug}
          onChange={(e) => {
            setSlug(e.target.value);
            setSlugEdited(true);
          }}
          required
        />
        <p id={hintId} className="text-sm text-muted">
          Lower-case letters, numbers and dashes, with at least one letter or dash. Used for the
          branch: feature/&lt;slug&gt;
        </p>
      </div>
      {create.isError && <p className="text-sm text-danger">{create.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button
          variant="primary"
          type="submit"
          loading={create.isPending}
          disabled={!isValidSlug(slug)}
        >
          Add
        </Button>
      </div>
    </form>
  );
}
