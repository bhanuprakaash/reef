import { useState } from 'react';
import { useCreateWorkItem } from '@/modules/work-items/api/create-work-item';
import { Button } from '@/components/button';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';
import type { WorkItem } from '@/types/api';

type Props = { projectId: number; onClose: () => void; onCreated?: (item: WorkItem) => void };

export function AddWorkItemForm({ projectId, onClose, onCreated }: Props) {
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const create = useCreateWorkItem();

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
        aria-label="Work item title"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        required
      />
      <TextArea
        className="w-full"
        rows={3}
        placeholder="Description"
        aria-label="Work item description"
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
