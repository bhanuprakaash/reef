import { useState } from 'react';
import { useUpdateWorkItem } from '@/modules/work-items/api/update-work-item';
import type { WorkItem } from '@/types/api';
import { Button } from '@/components/button';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

export function WorkItemForm({ item, onDone }: { item: WorkItem; onDone: () => void }) {
  const [title, setTitle] = useState(item.title);
  const [description, setDescription] = useState(item.description);
  const update = useUpdateWorkItem(item.id);

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        update.mutate({ ...item, title, description }, { onSuccess: onDone });
      }}
    >
      <label className="block space-y-1">
        <span className="text-sm text-muted">Title</span>
        <TextField
          className="w-full"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          required
        />
      </label>
      <label className="block space-y-1">
        <span className="text-sm text-muted">Description</span>
        <TextArea
          className="w-full"
          rows={4}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </label>
      {update.isError && <p className="text-sm text-danger">{update.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onDone}>
          Cancel
        </Button>
        <Button variant="primary" type="submit" loading={update.isPending}>
          Save
        </Button>
      </div>
    </form>
  );
}
