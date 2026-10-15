import { useUpdateFeature } from '@/modules/features/api/update-feature';
import { STAGES } from '@/modules/features/utils/stages';
import type { Feature } from '@/types/api';
import { Select } from '@/components/select';

export function StagePicker({ feature }: { feature: Feature }) {
  const update = useUpdateFeature(feature.id);
  const stages = STAGES.some((s) => s.value === feature.stage)
    ? STAGES
    : [...STAGES, { value: feature.stage, label: feature.stage }];

  return (
    <label className="relative inline-flex h-9 items-center rounded-pill border border-line px-3.5 text-[13px] text-soft focus-within:ring-3 focus-within:ring-ring">
      Stage: {stages.find((s) => s.value === feature.stage)?.label} ▾
      <Select
        aria-label="Stage"
        className="absolute inset-0 h-full w-full cursor-pointer opacity-0"
        value={feature.stage}
        disabled={update.isPending}
        onChange={(e) =>
          update.mutate({ name: feature.name, goal: feature.goal, stage: e.target.value })
        }
      >
        {stages.map((s) => (
          <option key={s.value} value={s.value}>
            {s.label}
          </option>
        ))}
      </Select>
    </label>
  );
}
