import { Link } from 'react-router';
import type { PullRequest } from '@/modules/channels/utils/pull-request';
import type { Channel } from '@/types/api';
import { Button } from '@/components/button';
import { Chip } from '@/components/chip';
import { GroupAvatar } from '@/components/creature';

type Props = {
  channel: Channel;
  scope: string;
  roles: string[];
  pullRequest: PullRequest | null;
  diff?: { added: number; removed: number };
  wrapDisabled: boolean;
  onWrap: () => void;
  onInspector: () => void;
  onTeam: () => void;
  onTerminal?: () => void;
};

const iconButton = 'flex size-9 items-center justify-center rounded-field text-soft hover:bg-card';

export function ChannelHeader({
  channel,
  scope,
  roles,
  pullRequest,
  diff,
  wrapDisabled,
  onWrap,
  onInspector,
  onTeam,
  onTerminal,
}: Props) {
  return (
    <header className="flex min-h-14 flex-wrap items-center gap-2.5 border-b border-hairline py-2 pl-[22px] pr-4">
      <GroupAvatar roles={roles} size={26} />
      <h1 className="text-base font-normal">
        <span aria-hidden="true">#</span> {channel.name}
      </h1>
      {channel.featureId === null ? (
        <span className="text-[13px] text-muted">{scope}</span>
      ) : (
        <Link
          to={`/features/${channel.featureId}`}
          className="text-[13px] text-muted hover:text-fg"
        >
          {scope}
        </Link>
      )}
      {pullRequest && (
        <a href={pullRequest.url} target="_blank" rel="noreferrer">
          <Chip>
            PR #{pullRequest.number} {pullRequest.state}
          </Chip>
        </a>
      )}
      <div className="ml-auto flex items-center gap-0.5">
        {onTerminal && (
          <button type="button" aria-label="Terminal" onClick={onTerminal} className={iconButton}>
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.6"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <rect x="3" y="4" width="18" height="16" rx="3" />
              <path d="M7 9l3 3-3 3M13 15h4" />
            </svg>
          </button>
        )}
        {diff && channel.featureId !== null && (
          <Link
            to={`/features/${channel.featureId}`}
            aria-label={`Diff +${diff.added} -${diff.removed}`}
            className="flex h-9 items-center gap-1.5 rounded-field px-2.5 text-[13px] text-soft hover:bg-card"
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
            <span className="text-done">+{diff.added}</span>
            <span className="text-failed">−{diff.removed}</span>
          </Link>
        )}
        <Button variant="ghost" type="button" onClick={onInspector}>
          Inspector
        </Button>
        <Button variant="ghost" type="button" onClick={onTeam} className="min-[1100px]:hidden">
          Team
        </Button>
        {channel.featureId !== null && (
          <Button
            variant="primary"
            type="button"
            className="ml-1.5"
            disabled={wrapDisabled}
            onClick={onWrap}
          >
            Wrap up
          </Button>
        )}
      </div>
    </header>
  );
}
