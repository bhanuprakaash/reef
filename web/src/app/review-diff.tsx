import { useState } from 'react';
import { Link } from 'react-router';
import { BackButton } from '@/components/back-button';
import { useSpaces } from '@/lib/api/get-spaces';
import { useBots } from '@/lib/api/get-bots';
import { type DiffScope, useDiff } from '@/modules/work-items/api/get-diff';
import { useWorkItem } from '@/modules/work-items/api/get-work-item';
import { DiffPanel } from '@/modules/work-items/components/diff-panel';
import { useProjects } from '@/modules/projects/api/get-projects';

const iconButton = 'flex size-9 items-center justify-center rounded-field text-soft hover:bg-card';

const confirmDiscard = (drafts: number) =>
  drafts === 0 || confirm(`Discard ${drafts} draft comment${drafts === 1 ? '' : 's'}?`);

type Props = { id: number; onClose?: () => void; defaultScope?: DiffScope };

export function ReviewDiff({ id, onClose, defaultScope = 'full' }: Props) {
  const [scope, setScope] = useState(defaultScope);
  const workItem = useWorkItem(id);
  const diff = useDiff(id, scope);
  const spaces = useSpaces();
  const bots = useBots();
  const projects = useProjects();

  const latest = spaces.data?.filter((c) => c.workItemId === id).at(-1);

  if (workItem.isPending || diff.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (workItem.isError) return <p className="text-sm text-danger">{workItem.error.message}</p>;
  if (diff.isError) return <p className="text-sm text-danger">{diff.error.message}</p>;

  return (
    <DiffPanel
      files={diff.data}
      workItemId={id}
      workItemName={workItem.data.title}
      branch={`work-item/${id}`}
      baseBranch={
        projects.data?.find((p) => p.id === workItem.data.projectId)?.baseBranch || 'main'
      }
      scope={scope}
      onScopeChange={setScope}
      developerLook={bots.data?.find(
        (b) => b.name === 'developer' && latest?.members.includes(b.id),
      )}
      leading={!onClose && <BackButton fallback={`/work-items/${id}`} />}
      actions={
        onClose &&
        ((drafts) => (
          <>
            <Link
              to={`/work-items/${id}/review`}
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
