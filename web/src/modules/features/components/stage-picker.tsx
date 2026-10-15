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
    <Select
      variant="pill"
      aria-label="Stage"
      prefix="Stage: "
      value={feature.stage}
      disabled={update.isPending}
      onChange={(stage) => update.mutate({ name: feature.name, goal: feature.goal, stage })}
      options={stages}
    />
  );
}
