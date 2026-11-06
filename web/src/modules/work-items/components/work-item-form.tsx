import { useState } from 'react';
import { useUpdateWorkItem } from '@/modules/work-items/api/update-work-item';
import type { WorkItem } from '@/types/api';
import { useBots } from '@/lib/api/get-bots';
import { Button } from '@/components/button';
import { Select } from '@/components/select';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

export function WorkItemForm({ item, onDone }: { item: WorkItem; onDone: () => void }) {
  const [title, setTitle] = useState(item.title);
  const [description, setDescription] = useState(item.description);
  const [ownerBotId, setOwnerBotId] = useState(item.ownerBotId);
  const [participantBotIds, setParticipantBotIds] = useState(item.participantBotIds);
  const bots = useBots();
  const update = useUpdateWorkItem(item.id);

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        update.mutate(
          {
            ...item,
            acceptanceLines: undefined,
            title,
            description,
            ownerBotId,
            participantBotIds,
          },
          { onSuccess: onDone },
        );
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
      {item.parentId === null && item.status === 'backlog' && (
        <>
          <div className="space-y-1">
            <span className="text-sm text-muted">Owner</span>
            <Select
              className="w-full"
              aria-label="Owner"
              value={ownerBotId === null ? '' : String(ownerBotId)}
              onChange={(v) => setOwnerBotId(v === '' ? null : Number(v))}
              options={[
                { value: '', label: 'No owner' },
                ...(bots.data ?? []).map((b) => ({ value: String(b.id), label: b.name })),
              ]}
            />
          </div>
          <div className="space-y-1">
            <span className="text-sm text-muted">Participants</span>
            <ul className="space-y-1">
              {(bots.data ?? []).map((b) => (
                <li key={b.id} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    aria-label={`Include ${b.name}`}
                    checked={participantBotIds.includes(b.id)}
                    onChange={(e) =>
                      setParticipantBotIds(
                        e.target.checked
                          ? [...participantBotIds, b.id]
                          : participantBotIds.filter((id) => id !== b.id),
                      )
                    }
                  />
                  {b.name}
                </li>
              ))}
            </ul>
          </div>
        </>
      )}
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
