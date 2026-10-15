import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { PullRequestIcon } from '@/modules/channels/components/pull-request-icon';
import type { PullRequest } from '@/modules/channels/utils/pull-request';
import type { Channel } from '@/types/api';
import { Button } from '@/components/button';
import { Chip } from '@/components/chip';
import { GroupAvatar } from '@/components/creature';

type Props = {
  channel: Channel;
  scope: ReactNode;
  roles: string[];
  faces: Record<string, string | undefined>;
  pullRequest: PullRequest | null;
  diff?: { added: number; removed: number };
  onDiff?: () => void;
  diffOpen?: boolean;
  wrapDisabled: boolean;
  onWrap: () => void;
  promoteDisabled: boolean;
  onPromote: () => void;
  onInspector: () => void;
  onTeam: () => void;
  browserOpen?: boolean;
};

const phoneIcon = 'max-md:size-11 max-md:px-0';

export function ChannelHeader({
  channel,
  scope,
  roles,
  faces,
  pullRequest,
  diff,
  onDiff,
  diffOpen,
  wrapDisabled,
  onWrap,
  promoteDisabled,
  onPromote,
  onInspector,
  onTeam,
  browserOpen,
}: Props) {
  return (
    <header className="flex min-h-14 flex-wrap items-center gap-2.5 border-b border-hairline py-2 pl-2 pr-2 md:pl-[22px] md:pr-4 in-data-desktop:pr-[88px]!">
      <Link
        to="/channels"
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
      <GroupAvatar roles={roles} faces={faces} size={26} />
      <div className="flex min-w-0 flex-1 flex-col md:flex-none md:flex-row md:items-center md:gap-2.5">
        <h1 className="truncate text-base font-normal">
          <span aria-hidden="true">#</span>
          {channel.name}
        </h1>
        {channel.featureId === null ? (
          <span className="flex min-w-0 items-center gap-2 text-[13px] text-muted">{scope}</span>
        ) : (
          <Link
            to={`/features/${channel.featureId}`}
            className="flex min-w-0 items-center gap-2 text-[13px] text-muted hover:text-fg"
          >
            {scope}
          </Link>
        )}
      </div>
      {pullRequest && (
        <a href={pullRequest.url} target="_blank" rel="noreferrer">
          <Chip>
            <PullRequestIcon className="text-done" />
            PR #{pullRequest.number} {pullRequest.state}
          </Chip>
        </a>
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
          className={`${browserOpen ? 'min-[1540px]:hidden' : 'min-[1100px]:hidden'} ${phoneIcon}`}
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
        {channel.featureId !== null ? (
          <Button
            variant="primary"
            type="button"
            className="ml-1.5 max-md:hidden"
            disabled={wrapDisabled}
            onClick={onWrap}
          >
            Wrap up
          </Button>
        ) : (
          <Button type="button" className="ml-1.5" disabled={promoteDisabled} onClick={onPromote}>
            Promote to feature
          </Button>
        )}
      </div>
    </header>
  );
}
