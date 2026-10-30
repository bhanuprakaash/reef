import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { PullRequestIcon } from '@/modules/spaces/components/pull-request-icon';
import type { PullRequest } from '@/modules/spaces/utils/pull-request';
import type { SpaceDetail } from '@/types/api';
import { Button, buttonClass } from '@/components/button';
import { Chip } from '@/components/chip';
import { GroupAvatar, type Faces } from '@/components/creature';

type Props = {
  space: SpaceDetail;
  scope: ReactNode;
  status?: string;
  roles: number[];
  faces: Faces;
  working: number | null;
  pullRequest: PullRequest | null;
  onPullRequest: () => void;
  pullRequestOpen: boolean;
  diff?: { added: number; removed: number };
  onDiff?: () => void;
  diffOpen?: boolean;
  wrapDisabled: boolean;
  onWrap: () => void;
  attachDisabled: boolean;
  onAttach: () => void;
  onAddBot: () => void;
  onInspector: () => void;
  onTeam: () => void;
};

const phoneIcon = 'max-md:size-11 max-md:px-0';

export function SpaceHeader({
  space,
  scope,
  status,
  roles,
  faces,
  working,
  pullRequest,
  onPullRequest,
  pullRequestOpen,
  diff,
  onDiff,
  diffOpen,
  wrapDisabled,
  onWrap,
  attachDisabled,
  onAttach,
  onAddBot,
  onInspector,
  onTeam,
}: Props) {
  return (
    <header className="flex min-h-14 flex-wrap items-center gap-2.5 border-b border-hairline py-2 pl-2 pr-2 md:pl-[22px] md:pr-4 in-data-desktop:pr-[88px]!">
      <Link
        to="/spaces"
        aria-label="Back"
        className="flex size-11 items-center justify-center rounded-field text-soft md:hidden"
      >
        <svg
          width="22"
          height="22"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M15 5l-7 7 7 7" />
        </svg>
      </Link>
      <button
        type="button"
        onClick={onTeam}
        aria-label="Show team"
        className="flex shrink-0 items-center justify-center rounded-field max-md:size-11"
      >
        <GroupAvatar roles={roles} faces={faces} size={26} working={working} />
      </button>
      <div className="flex min-w-0 flex-1 flex-col md:flex-none md:flex-row md:items-center md:gap-2.5">
        <h1 className="truncate text-base font-normal">
          <span aria-hidden="true">#</span>
          {space.name}
        </h1>
        {space.workItemId === null ? (
          <span className="flex min-w-0 items-center gap-2 text-[13px] text-muted">{scope}</span>
        ) : (
          <Link
            to={`/work-items/${space.workItemId}`}
            className="flex min-w-0 items-center gap-2 text-[13px] text-muted hover:text-fg"
          >
            {scope}
          </Link>
        )}
        {space.kind === 'work_item' && status && (
          <span className="flex items-center gap-2 max-md:mt-1">
            <Chip>{status}</Chip>
            <Chip>
              Budget {space.tokensUsed.toLocaleString('en-US')} /{' '}
              {space.tokenBudget.toLocaleString('en-US')}
            </Chip>
          </span>
        )}
      </div>
      {pullRequest && (
        <button type="button" onClick={onPullRequest} aria-pressed={pullRequestOpen}>
          <Chip>
            <PullRequestIcon className="text-done" />
            PR #{pullRequest.number} {pullRequest.state}
          </Chip>
        </button>
      )}
      <div className="flex items-center gap-0.5 md:ml-auto">
        {diff && onDiff && (
          <button
            type="button"
            onClick={onDiff}
            aria-pressed={diffOpen}
            aria-label={`Diff +${diff.added} -${diff.removed}`}
            className="flex h-11 items-center gap-1.5 rounded-field px-2.5 text-[13px] text-soft hover:bg-card aria-pressed:bg-bubble aria-pressed:text-fg md:h-9"
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.6"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M12 6v6M9 9h6M9 17h6" />
              <rect x="3" y="3" width="18" height="18" rx="4" />
            </svg>
            <span className="text-done max-md:hidden">+{diff.added}</span>
            <span className="text-failed max-md:hidden">−{diff.removed}</span>
          </button>
        )}
        <Button
          variant="ghost"
          type="button"
          onClick={onInspector}
          aria-label="Inspector"
          className={phoneIcon}
        >
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.7"
            strokeLinecap="round"
            aria-hidden="true"
            className="md:hidden"
          >
            <path d="M5 20V10M12 20V4M19 20v-7" />
          </svg>
          <span className="max-md:hidden">Inspector</span>
        </Button>
        <Button
          variant="ghost"
          type="button"
          onClick={onTeam}
          aria-label="Team"
          className={phoneIcon}
        >
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.7"
            strokeLinecap="round"
            aria-hidden="true"
            className="md:hidden"
          >
            <circle cx="12" cy="12" r="9" />
            <path d="M12 11v6M12 7.5v.5" />
          </svg>
          <span className="max-md:hidden">Team</span>
        </Button>
        {space.workItemId !== null && (
          <Link to={`/work-items/${space.workItemId}`} className={`${buttonClass()} ml-1.5`}>
            Open Work item
          </Link>
        )}
        {space.workItemId !== null
          ? space.closedAt === null && (
              <Button
                variant="primary"
                type="button"
                className="ml-1.5 max-md:hidden"
                disabled={wrapDisabled}
                onClick={onWrap}
              >
                Wrap up
              </Button>
            )
          : space.kind === 'free' && (
              <Button type="button" className="ml-1.5" disabled={attachDisabled} onClick={onAttach}>
                Attach to Work item
              </Button>
            )}
        {(space.kind === 'free' || (space.kind === 'conversation' && space.movedTo === null)) && (
          <Button
            variant="ghost"
            type="button"
            aria-label="Add Bot"
            className={phoneIcon}
            disabled={attachDisabled}
            onClick={onAddBot}
          >
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.7"
              strokeLinecap="round"
              aria-hidden="true"
              className="md:hidden"
            >
              <circle cx="9" cy="8" r="3.5" />
              <path d="M3 19c0-3.3 2.7-6 6-6s6 2.7 6 6M19 8v6M16 11h6" />
            </svg>
            <span className="max-md:hidden">Add Bot</span>
          </Button>
        )}
      </div>
    </header>
  );
}
