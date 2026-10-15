import { useState } from 'react';
import { useUpdateFeature } from '@/modules/features/api/update-feature';
import type { Feature } from '@/types/api';
import { Button } from '@/components/button';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

export function FeatureForm({ feature, onDone }: { feature: Feature; onDone: () => void }) {
  const [name, setName] = useState(feature.name);
  const [goal, setGoal] = useState(feature.goal);
  const update = useUpdateFeature(feature.id);

  return (
    <form
      className="max-w-xl space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        update.mutate({ name, goal, stage: feature.stage }, { onSuccess: onDone });
      }}
    >
      <label className="block space-y-1">
        <span className="text-sm text-muted">Name</span>
        <TextField
          className="w-full"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
      </label>
      <label className="block space-y-1">
        <span className="text-sm text-muted">About</span>
        <TextArea
          className="w-full"
          rows={4}
          value={goal}
          onChange={(e) => setGoal(e.target.value)}
        />
      </label>
      <div className="flex items-center gap-3">
        <Button variant="primary" type="submit" loading={update.isPending}>
          Save
        </Button>
        <Button type="button" onClick={onDone}>
          Cancel
        </Button>
        {update.isError && <span className="text-sm text-danger">{update.error.message}</span>}
      </div>
    </form>
  );
}
