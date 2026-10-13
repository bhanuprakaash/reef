import { useState } from 'react';
import { useUpdateFeature } from '@/modules/features/api/update-feature';
import { STAGES } from '@/modules/features/utils/stages';
import type { Feature } from '@/types/api';
import { buttonClass, errorClass, inputClass, mutedClass } from '@/utils/classes';

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
        <span className={mutedClass}>Name</span>
        <input
          className={`${inputClass} w-full`}
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
      </label>
      <label className="block space-y-1">
        <span className={mutedClass}>About</span>
        <textarea
          className={`${inputClass} w-full`}
          rows={4}
          value={goal}
          onChange={(e) => setGoal(e.target.value)}
        />
      </label>
      <label className="block space-y-1">
        <span className={mutedClass}>Stage</span>
        <select
          className={`${inputClass} block`}
          value={stage}
          onChange={(e) => setStage(e.target.value)}
        >
          {stages.map((s) => (
            <option key={s.value} value={s.value}>
              {s.label}
            </option>
          ))}
        </select>
      </label>
      <div className="flex items-center gap-3">
        <button type="submit" className={buttonClass} disabled={update.isPending}>
          Save
        </button>
        {update.isSuccess && <span className={mutedClass}>Saved</span>}
        {update.isError && <span className={errorClass}>{update.error.message}</span>}
      </div>
    </form>
  );
}
