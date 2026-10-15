import { useParams } from 'react-router';
import { useChannels } from '@/lib/api/get-channels';
import { useTeams } from '@/lib/api/get-teams';
import { useDiff } from '@/modules/features/api/get-diff';
import { useFeature } from '@/modules/features/api/get-feature';
import { DiffPanel } from '@/modules/features/components/diff-panel';
import { roleFaces } from '@/utils/role-faces';

export function ReviewPage() {
  const id = Number(useParams().id);
  const feature = useFeature(id);
  const diff = useDiff(id);
  const channels = useChannels();
  const teams = useTeams();

  const latestTeam = channels.data?.filter((c) => c.featureId === id).at(-1)?.team;

  if (feature.isPending || diff.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (feature.isError) return <p className="text-sm text-danger">{feature.error.message}</p>;
  if (diff.isError) return <p className="text-sm text-danger">{diff.error.message}</p>;

  return (
    <div className="-m-4 flex h-screen flex-col md:-m-6">
      <DiffPanel
        files={diff.data}
        featureId={id}
        featureName={feature.data.name}
        branch={`feature/${feature.data.slug}`}
        developerFace={roleFaces(teams.data, latestTeam).developer}
      />
    </div>
  );
}
