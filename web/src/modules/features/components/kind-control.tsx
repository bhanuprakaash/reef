import { SegmentedControl } from '@/components/segmented-control';
import type { FeatureKind } from '@/types/api';

export function KindControl({
  value,
  onChange,
}: {
  value: FeatureKind;
  onChange: (kind: FeatureKind) => void;
}) {
  return (
    <SegmentedControl
      label="Kind"
      options={[
        { value: 'journey', label: 'Journey (stages)' },
        { value: 'fixes', label: 'Fixes (task list)' },
      ]}
      value={value}
      onChange={onChange}
    />
  );
}
