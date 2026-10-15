import { useParams } from 'react-router';
import { useDiff } from '@/modules/features/api/get-diff';
import { useFeature } from '@/modules/features/api/get-feature';
import { DiffPanel } from '@/modules/features/components/diff-panel';

export function ReviewPage() {
  const id = Number(useParams().id);
  const feature = useFeature(id);
  const diff = useDiff(id);

  if (feature.isPending || diff.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (feature.isError) return <p className="text-sm text-danger">{feature.error.message}</p>;
  if (diff.isError) return <p className="text-sm text-danger">{diff.error.message}</p>;

  return (
    <div className="-m-6 flex h-screen flex-col">
      <DiffPanel
        files={diff.data}
        featureId={id}
        featureName={feature.data.name}
        branch={`feature/${feature.data.slug}`}
      />
    </div>
  );
}
