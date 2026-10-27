import { useState } from 'react';
import { Link } from 'react-router';
import { useCreateFeature } from '@/modules/features/api/create-feature';
import { useSubItems } from '@/modules/features/api/get-sub-items';
import { statusLabel } from '@/modules/features/utils/status-label';
import type { WorkItem } from '@/types/api';
import { Button } from '@/components/button';
import { Card } from '@/components/card';
import { EmptyState } from '@/components/empty-state';
import { TextField } from '@/components/text-field';

export function SubItems({ parent }: { parent: WorkItem }) {
  const [title, setTitle] = useState('');
  const subs = useSubItems(parent.id);
  const create = useCreateFeature();

  return (
    <Card title="Sub-items">
      {subs.isError && <p className="text-sm text-danger">{subs.error.message}</p>}
      {subs.data?.length === 0 && (
        <EmptyState
          title="No sub-items yet"
          body="Split the work into smaller steps."
          role="developer"
          compact
        />
      )}
      <ul className="text-[13px]">
        {subs.data?.map((s) => (
          <li key={s.id}>
            <Link
              to={`/features/${s.id}`}
              className="flex min-h-10 items-center gap-2.5 text-soft hover:text-fg"
            >
              {s.title}
              <span className="ml-auto text-xs text-muted">{statusLabel(s.status)}</span>
            </Link>
          </li>
        ))}
      </ul>
      <form
        className="mt-3 flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate(
            { projectId: parent.projectId, parentId: parent.id, type: 'task', title },
            { onSuccess: () => setTitle('') },
          );
        }}
      >
        <TextField
          className="min-w-0 flex-1"
          placeholder="New sub-item"
          aria-label="New sub-item"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          required
        />
        <Button type="submit" loading={create.isPending}>
          Add
        </Button>
      </form>
      {create.isError && <p className="mt-2 text-sm text-danger">{create.error.message}</p>}
    </Card>
  );
}
