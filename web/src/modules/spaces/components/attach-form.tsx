import { useState } from 'react';
import { useAttachSpace } from '@/modules/spaces/api/attach-space';
import type { Space, WorkItem } from '@/types/api';
import { Button } from '@/components/button';
import { Select } from '@/components/select';

export function AttachForm({
  space,
  workItems,
  onClose,
}: {
  space: Space;
  workItems: WorkItem[];
  onClose: () => void;
}) {
  const [workItemId, setWorkItemId] = useState('');
  const attach = useAttachSpace(space.id);
  const choices = workItems.filter((w) => w.parentId === null);

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        attach.mutate(Number(workItemId), { onSuccess: onClose });
      }}
    >
      <p className="-mt-2.5 text-[13px] text-muted">{space.name} becomes the work item's Space</p>
      <label className="block">
        <span className="mb-1.5 block text-[12.5px] text-muted">Work item</span>
        <Select
          className="w-full"
          aria-label="Work item"
          value={workItemId}
          onChange={setWorkItemId}
          options={[
            { value: '', label: 'Pick a work item', disabled: true },
            ...choices.map((w) => ({ value: String(w.id), label: w.title })),
          ]}
        />
      </label>
      <p className="text-[13px] text-muted">
        The Space's saved items move to the work item. A work item has one active Space.
      </p>
      {attach.isError && <p className="text-sm text-danger">{attach.error.message}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button
          variant="primary"
          type="submit"
          disabled={workItemId === ''}
          loading={attach.isPending}
        >
          Attach
        </Button>
      </div>
    </form>
  );
}
