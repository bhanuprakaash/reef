import { Link } from 'react-router';
import { BackButton } from '@/components/back-button';
import { useChannels } from '@/lib/api/get-channels';
import { useTeams } from '@/lib/api/get-teams';
import { useDiff } from '@/modules/features/api/get-diff';
import { useFeature } from '@/modules/features/api/get-feature';
import { DiffPanel } from '@/modules/features/components/diff-panel';
import { roleFaces } from '@/utils/role-faces';

const iconButton = 'flex size-9 items-center justify-center rounded-field text-soft hover:bg-card';

const confirmDiscard = (drafts: number) =>
  drafts === 0 || confirm(`Discard ${drafts} draft comment${drafts === 1 ? '' : 's'}?`);

export function ReviewDiff({ id, onClose }: { id: number; onClose?: () => void }) {
  const feature = useFeature(id);
  const diff = useDiff(id);
  const channels = useChannels();
  const teams = useTeams();

  const latestTeam = channels.data?.filter((c) => c.featureId === id).at(-1)?.team;

  if (feature.isPending || diff.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (feature.isError) return <p className="text-sm text-danger">{feature.error.message}</p>;
  if (diff.isError) return <p className="text-sm text-danger">{diff.error.message}</p>;

  return (
    <DiffPanel
      files={diff.data}
      featureId={id}
      featureName={feature.data.name}
      branch={`feature/${feature.data.slug}`}
      developerFace={roleFaces(teams.data, latestTeam).developer}
      leading={!onClose && <BackButton fallback={`/features/${id}`} />}
      actions={
        onClose &&
        ((drafts) => (
          <>
            <Link
              to={`/features/${id}/review`}
              aria-label="Expand diff"
              className={iconButton}
              onClick={(e) => {
                if (!confirmDiscard(drafts)) e.preventDefault();
              }}
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                <path d="M14 4h6v6M10 20H4v-6M20 4l-7 7M4 20l7-7" />
              </svg>
            </Link>
            <button
              type="button"
              aria-label="Close diff"
              onClick={() => confirmDiscard(drafts) && onClose()}
              className={iconButton}
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                aria-hidden="true"
              >
                <path d="M6 6l12 12M18 6L6 18" />
              </svg>
            </button>
          </>
        ))
      }
    />
  );
}
