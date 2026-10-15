import { useState } from 'react';
import { useUpdateFeature } from '@/modules/features/api/update-feature';
import { STAGES } from '@/modules/features/utils/stages';
import type { Feature } from '@/types/api';
import { Button } from '@/components/button';
import { Select } from '@/components/select';
import { TextArea } from '@/components/text-area';
import { TextField } from '@/components/text-field';

export function FeatureForm({ feature }: { feature: Feature }) {
  const [name, setName] = useState(feature.name);
  const [goal, setGoal] = useState(feature.goal);
  const [stage, setStage] = useState(feature.stage);
  const update = useUpdateFeature(feature.id);
  const stages = STAGES.some((s) => s.value === stage)
    ? STAGES
    : [...STAGES, { value: stage, label: stage }];

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        update.mutate({ name, goal, stage });
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
      <label className="block space-y-1">
        <span className="text-sm text-muted">Stage</span>
        <Select className="block" value={stage} onChange={(e) => setStage(e.target.value)}>
          {stages.map((s) => (
            <option key={s.value} value={s.value}>
              {s.label}
            </option>
          ))}
        </Select>
      </label>
      <div className="flex items-center gap-3">
        <Button variant="primary" type="submit" loading={update.isPending}>
          Save
        </Button>
        {update.isSuccess && <span className="text-sm text-muted">Saved</span>}
        {update.isError && <span className="text-sm text-danger">{update.error.message}</span>}
      </div>
    </form>
  );
}
