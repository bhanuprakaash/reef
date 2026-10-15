import { StageChip } from '@/components/stage-chip';
import { STAGES } from '@/modules/features/utils/stages';
import type { Feature, Task } from '@/types/api';

export function featureStage(feature: Feature, tasks: Task[]) {
  const s = STAGES.find((x) => x.value === feature.stage);
  const mine = tasks.filter((t) => t.featureId === feature.id);
  return {
    label:
      feature.kind === 'fixes'
        ? `${mine.filter((t) => t.status === 'done').length}/${mine.length}`
        : (s?.label ?? feature.stage),
    color: s?.color ?? 'var(--color-muted)',
  };
}

export function FeatureStageChip({ feature, tasks }: { feature: Feature; tasks: Task[] }) {
  const { label, color } = featureStage(feature, tasks);
  return <StageChip label={label} color={color} />;
}
