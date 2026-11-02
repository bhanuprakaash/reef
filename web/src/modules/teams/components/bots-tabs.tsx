import { useSearchParams } from 'react-router';
import { SegmentedControl } from '@/components/segmented-control';

export function BotsTabs() {
  const [params, setParams] = useSearchParams();
  const tab = params.get('tab') === 'crews' ? 'crews' : 'bots';
  return (
    <SegmentedControl
      label="Bots page"
      className="self-start"
      value={tab}
      onChange={(next) => setParams(next === 'crews' ? { tab: 'crews' } : {})}
      options={[
        { value: 'bots', label: 'Bots' },
        { value: 'crews', label: 'Crews' },
      ]}
    />
  );
}
