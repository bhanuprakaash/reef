import { useState } from 'react';
import { useUpdateFeature } from '@/modules/features/api/update-feature';
import type { WorkItem } from '@/types/api';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { EmptyState } from '@/components/empty-state';
import { StatusDot } from '@/components/status-dot';
import { TextField } from '@/components/text-field';

export function AcceptanceLines({ item }: { item: WorkItem }) {
  const [text, setText] = useState('');
  const update = useUpdateFeature(item.id);
  const lines = item.acceptanceLines;
  const ticked = lines.filter((l) => l.checked).length;

  return (
    <Card
      title="Acceptance lines"
      aside={
        lines.length > 0 && (
          <span className="ml-auto text-xs text-muted tabular-nums">
            {ticked}/{lines.length}
          </span>
        )
      }
    >
      {lines.length === 0 ? (
        <EmptyState
          title="No acceptance lines yet"
          body="What must hold for this to be done."
          role="tester"
          compact
        />
      ) : (
        <ul className="text-sm">
          {lines.map((l) => (
            <li key={l.position} className="flex items-start gap-3 py-2">
              <span className="pt-[7px]">
                <StatusDot
                  status={l.checked ? 'done' : 'asleep'}
                  label={l.checked ? 'Ticked' : 'Not ticked'}
                />
              </span>
              <span className="min-w-0 flex-1 pt-0.5">
                {l.text}
                {l.note && <span className="block text-xs text-muted">{l.note}</span>}
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="shrink-0"
                disabled={update.isPending}
                aria-label={`Remove line ${l.position}`}
                onClick={() =>
                  update.mutate({
                    ...item,
                    acceptanceLines: lines.filter((x) => x.position !== l.position),
                  })
                }
              >
                Remove
              </Button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="mt-3 flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          update.mutate(
            {
              ...item,
              acceptanceLines: [
                ...lines,
                { position: lines.length + 1, text, checked: false, note: '' },
              ],
            },
            { onSuccess: () => setText('') },
          );
        }}
      >
        <TextField
          className="min-w-0 flex-1"
          placeholder="New acceptance line"
          aria-label="New acceptance line"
          value={text}
          onChange={(e) => setText(e.target.value)}
          required
        />
        <Button type="submit" loading={update.isPending}>
          Add
        </Button>
      </form>
      {update.isError && <p className="mt-2 text-sm text-danger">{update.error.message}</p>}
    </Card>
  );
}
