import { useState } from 'react';
import { useCreateFeature } from '@/modules/features/api/create-feature';
import { Button } from '@/components/button';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';
import type { WorkItem } from '@/types/api';

type Props = { projectId: number; onClose: () => void; onCreated?: (item: WorkItem) => void };

export function AddFeatureForm({ projectId, onClose, onCreated }: Props) {
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const create = useCreateFeature();

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { projectId, title, description },
          {
            onSuccess: (item) => {
              onClose();
              onCreated?.(item);
            },
          },
        );
      }}
    >
      <TextField
        className="w-full"
        placeholder="Title"
        aria-label="Feature title"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        required
      />
      <TextArea
        className="w-full"
        rows={3}
        placeholder="Description"
        aria-label="Feature description"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
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
