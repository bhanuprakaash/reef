import { useRef, useState, type ReactNode } from 'react';
import { Link, useNavigate } from 'react-router';
import { canAddBot } from '@/modules/spaces/components/add-bot-form';
import { PullRequestIcon } from '@/modules/spaces/components/pull-request-icon';
import type { PullRequest } from '@/modules/spaces/utils/pull-request';
import type { Role, SpaceDetail } from '@/types/api';
import { Button } from '@/components/button';
import { Chip } from '@/components/chip';
import { Creature, GroupAvatar, faceOf, type Faces } from '@/components/creature';
import { Menu, MenuItem, MenuSeparator } from '@/components/menu';
import { StatusDot } from '@/components/status-dot';
import { useDismiss } from '@/lib/use-dismiss';

type Props = {
  readOnly?: boolean;
  space: SpaceDetail;
  scope: ReactNode;
  status?: string;
  roles: Role[];
  faces: Faces;
  working: number | null;
  pullRequest: PullRequest | null;
  onPullRequest: () => void;
  pullRequestOpen: boolean;
  diff?: { added: number; removed: number };
  onDiff?: () => void;
  diffOpen?: boolean;
  tools?: ReactNode;
  wrapDisabled: boolean;
  onWrap: () => void;
  attachDisabled: boolean;
  onAttach: () => void;
  closeDisabled: boolean;
  onClose: () => void;
  onAddBot: () => void;
  onNewChat: () => void;
  newChatDisabled: boolean;
  onInspector: () => void;
  onTeam: () => void;
};

function Icon({ children, size = 17 }: { children: ReactNode; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className="shrink-0"
    >
      {children}
    </svg>
  );
}

function usePopup() {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useDismiss(open, ref, () => setOpen(false));
  return { open, setOpen, ref };
}

type BotsPopupProps = Pick<Props, 'roles' | 'faces' | 'working' | 'onAddBot' | 'onTeam'> & {
  compact?: boolean;
  canAdd: boolean;
  addDisabled: boolean;
};

function BotsPopup({
  roles,
  faces,
  working,
  onAddBot,
  onTeam,
  compact,
  canAdd,
  addDisabled,
}: BotsPopupProps) {
  const { open, setOpen, ref } = usePopup();
  const pick = (action: () => void) => () => {
    setOpen(false);
    action();
  };
  return (
    <div ref={ref} className={`relative ${compact ? 'md:hidden' : 'max-md:hidden'}`}>
      {compact ? (
        <button
          type="button"
          aria-label="Show Bots"
          aria-haspopup="menu"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          className="flex size-11 items-center justify-center rounded-field"
        >
          <GroupAvatar
            roles={roles.map((r) => r.botId)}
            faces={faces}
            size={26}
            working={working}
          />
        </button>
      ) : (
        <Button
          variant="ghost"
          type="button"
          aria-label="Bots"
          aria-haspopup="menu"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          className="gap-1.5"
        >
          <Icon>
            <circle cx="9" cy="8" r="3.2" />
            <path d="M3 19c0-3.3 2.7-6 6-6s6 2.7 6 6M16 5.5a3 3 0 0 1 0 5.5M18 13.3c2 .8 3.3 2.7 3.3 5.2" />
          </Icon>
          Bots
          <span className="rounded-pill bg-bubble px-1.5 text-[11.5px] leading-[18px] text-soft">
            {roles.length}
          </span>
          <Icon size={14}>
            <path d="M7 10l5 5 5-5" />
          </Icon>
        </Button>
      )}
      {open && (
        <Menu className={`absolute top-full z-30 mt-1.5 w-60 ${compact ? 'left-0' : 'right-0'}`}>
          <div role="group" aria-label="Bots in this Space">
            <p className="px-2.5 pt-1.5 pb-1 text-[11px] tracking-wider text-muted uppercase">
              Bots · {roles.length}
            </p>
            {roles.map((r) => {
              const busy = r.botId === working;
              const state = busy ? 'working' : r.lead ? 'idle' : 'asleep';
              return (
                <div key={r.botId} className="flex items-center gap-2.5 px-2.5 py-1.5">
                  <Creature look={faceOf(faces, r.botId)} state={state} size={26} />
                  <span className="flex min-w-0 flex-col leading-tight">
                    <span className="truncate text-sm">{r.name}</span>
                    <span className="flex items-center gap-1.5 text-xs text-muted">
                      <StatusDot status={busy ? 'working' : r.lead ? 'done' : 'asleep'} />
                      {busy ? 'working' : r.lead ? 'coordinator' : 'asleep'}
                    </span>
                  </span>
                </div>
              );
            })}
          </div>
          <MenuSeparator />
          {canAdd && (
            <MenuItem
              disabled={addDisabled}
              className="disabled:text-faint"
              onClick={pick(onAddBot)}
            >
              <Icon>
                <circle cx="9" cy="8" r="3.5" />
                <path d="M3 19c0-3.3 2.7-6 6-6s6 2.7 6 6M19 8v6M16 11h6" />
              </Icon>
              Add Bot
            </MenuItem>
          )}
          <MenuItem onClick={pick(onTeam)}>
            <Icon>
              <circle cx="12" cy="12" r="3" />
              <path d="M12 3v2.5M12 18.5V21M3 12h2.5M18.5 12H21M5.6 5.6l1.8 1.8M16.6 16.6l1.8 1.8M5.6 18.4l1.8-1.8M16.6 7.4l1.8-1.8" />
            </Icon>
            Bot settings
          </MenuItem>
        </Menu>
      )}
    </div>
  );
}

export function SpaceHeader({
  readOnly = false,
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
  tools,
  wrapDisabled,
  onWrap,
  attachDisabled,
  onAttach,
  closeDisabled,
  onClose,
  onAddBot,
  onNewChat,
  newChatDisabled,
  onInspector,
  onTeam,
}: Props) {
  const { open: moreOpen, setOpen: setMoreOpen, ref: moreRef } = usePopup();
  const navigate = useNavigate();
  const workItem = space.workItemId !== null;
  const canWrap = workItem && space.closedAt === null;
  const canAttach = !workItem && space.kind === 'free';
  const canClose = canAttach && space.closedAt === null;
  const canAdd = !readOnly && canAddBot(space);
  const pick = (action: () => void) => () => {
    setMoreOpen(false);
    action();
  };
  return (
    <header className="flex min-h-14 flex-wrap items-center gap-2.5 border-b border-hairline py-2 pl-2 pr-2 md:pl-[22px] md:pr-4">
      <Link
        to="/spaces"
        aria-label="Back"
        className="flex size-11 items-center justify-center rounded-field text-soft md:hidden"
      >
        <Icon size={22}>
          <path d="M15 5l-7 7 7 7" />
        </Icon>
      </Link>
      <BotsPopup
        compact
        roles={roles}
        faces={faces}
        working={working}
        canAdd={canAdd}
        addDisabled={attachDisabled}
        onAddBot={onAddBot}
        onTeam={onTeam}
      />
      <span role="img" aria-label="Bots" className="flex shrink-0 max-md:hidden">
        <GroupAvatar roles={roles.map((r) => r.botId)} faces={faces} size={26} working={working} />
      </span>
      <div className="flex min-w-0 flex-1 flex-col md:gap-0.5">
        <div className="flex min-w-0 flex-col md:flex-row md:items-center md:gap-2.5">
          <h1 className="truncate text-base font-normal">{space.name}</h1>
          {!workItem && (
            <span className="flex min-w-0 items-center gap-2 text-[13px] text-muted">{scope}</span>
          )}
        </div>
        {workItem && (
          <div className="flex min-w-0 flex-wrap items-center gap-2 max-md:mt-1">
            <Link
              to={`/work-items/${space.workItemId}`}
              className="flex min-w-0 items-center gap-2 text-[13px] text-muted hover:text-fg"
            >
              {scope}
            </Link>
            {space.kind === 'work_item' && status && (
              <>
                <Chip>{status}</Chip>
                <Chip className="max-md:hidden">
                  Budget {space.tokensUsed.toLocaleString('en-US')} /{' '}
                  {space.tokenBudget.toLocaleString('en-US')}
                </Chip>
              </>
            )}
            {pullRequest && (
              <button
                type="button"
                onClick={onPullRequest}
                aria-pressed={pullRequestOpen}
                className="flex items-center max-md:h-11"
              >
                <Chip>
                  <PullRequestIcon className="text-done" />
                  PR #{pullRequest.number} {pullRequest.state}
                </Chip>
              </button>
            )}
            {diff && onDiff && (
              <button
                type="button"
                onClick={onDiff}
                aria-pressed={diffOpen}
                aria-label={`Diff +${diff.added} -${diff.removed}`}
                className="flex items-center max-md:h-11"
              >
                <Chip>
                  <Icon size={13}>
                    <path d="M12 6v6M9 9h6M9 17h6" />
                    <rect x="3" y="3" width="18" height="18" rx="4" />
                  </Icon>
                  <span className="text-done">+{diff.added}</span>
                  <span className="text-failed">−{diff.removed}</span>
                </Chip>
              </button>
            )}
          </div>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        {space.kind === 'conversation' && roles.length > 0 && (
          <Button
            variant="secondary"
            type="button"
            aria-label="New chat"
            disabled={newChatDisabled}
            onClick={onNewChat}
            className="max-md:size-11 max-md:rounded-field max-md:border-0 max-md:px-0"
          >
            <span className="max-md:hidden">
              <Icon size={16}>
                <path d="M12 5v14M5 12h14" />
              </Icon>
            </span>
            <span className="md:hidden">
              <Icon size={20}>
                <path d="M12 20a8 8 0 1 0-7.2-4.5L4 20l4.4-.8A8 8 0 0 0 12 20zM12 9v6M9 12h6" />
              </Icon>
            </span>
            <span className="max-md:hidden">New chat</span>
          </Button>
        )}
        <BotsPopup
          roles={roles}
          faces={faces}
          working={working}
          canAdd={canAdd}
          addDisabled={attachDisabled}
          onAddBot={onAddBot}
          onTeam={onTeam}
        />
        {tools && <div className="max-md:hidden">{tools}</div>}
        {canWrap && (
          <Button
            variant="primary"
            type="button"
            className="max-md:hidden"
            disabled={wrapDisabled}
            onClick={onWrap}
          >
            Wrap up
          </Button>
        )}
        <div ref={moreRef} className="relative">
          <Button
            variant="ghost"
            type="button"
            aria-label="More"
            aria-haspopup="menu"
            aria-expanded={moreOpen}
            onClick={() => setMoreOpen(!moreOpen)}
            className="max-md:size-11 max-md:px-0 md:w-9 md:px-0"
          >
            <Icon size={20}>
              <circle cx="5" cy="12" r="1.3" fill="currentColor" />
              <circle cx="12" cy="12" r="1.3" fill="currentColor" />
              <circle cx="19" cy="12" r="1.3" fill="currentColor" />
            </Icon>
          </Button>
          {moreOpen && (
            <Menu className="absolute top-full right-0 z-30 mt-1.5 w-56">
              {canWrap && (
                <MenuItem disabled={wrapDisabled} className="md:hidden" onClick={pick(onWrap)}>
                  <Icon>
                    <path d="M5 12.5l4.5 4.5L19 7.5" />
                  </Icon>
                  Wrap up
                </MenuItem>
              )}
              <MenuItem onClick={pick(onInspector)}>
                <Icon>
                  <path d="M5 20V10M12 20V4M19 20v-7" />
                </Icon>
                Inspector
              </MenuItem>
              {canAttach && (
                <MenuItem disabled={attachDisabled} onClick={pick(onAttach)}>
                  <Icon>
                    <path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1" />
                  </Icon>
                  Attach to Work item
                </MenuItem>
              )}
              {canClose && (
                <MenuItem disabled={closeDisabled} onClick={pick(onClose)}>
                  <Icon>
                    <path d="M5 12.5l4.5 4.5L19 7.5" />
                  </Icon>
                  Close Space
                </MenuItem>
              )}
              {workItem && (
                <MenuItem onClick={pick(() => void navigate(`/work-items/${space.workItemId}`))}>
                  <Icon>
                    <path d="M9 6l6 6-6 6" />
                  </Icon>
                  Open Work item
                </MenuItem>
              )}
            </Menu>
          )}
        </div>
      </div>
    </header>
  );
}
