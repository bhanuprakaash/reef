import { type CSSProperties, type ReactNode, useEffect, useState } from 'react';
import type { BrowserDriver } from '@/app/browser-pane';
import { Link } from 'react-router';
import { Dialog } from '@/components/dialog';
import { ResizeHandle } from '@/components/resize-handle';
import { useResizableWidth } from '@/lib/use-resizable-width';
import { useDirectMessages } from '@/modules/spaces/api/get-direct-messages';
import { useFailedTurns } from '@/modules/spaces/api/get-failed-turns';
import { useMessages } from '@/lib/api/get-messages';
import { useSpaceMembers } from '@/modules/spaces/api/get-space-members';
import { useSpaceTurns, useTurns } from '@/modules/spaces/api/get-turns';
import { useReopenSpace } from '@/modules/spaces/api/reopen-space';
import { useProposal } from '@/modules/spaces/api/get-proposal';
import { useWrapSpace } from '@/modules/spaces/api/wrap-space';
import { AddBotForm } from '@/modules/spaces/components/add-bot-form';
import { AttachForm } from '@/modules/spaces/components/attach-form';
import { SpaceHeader } from '@/modules/spaces/components/space-header';
import { CheckpointRows } from '@/modules/spaces/components/checkpoint-rows';
import { Composer } from '@/modules/spaces/components/composer';
import { CoordinationPanel } from '@/modules/spaces/components/coordination-panel';
import { DeniedToolRows } from '@/modules/spaces/components/denied-tool-rows';
import { FailedTurnRows } from '@/modules/spaces/components/failed-turn-rows';
import { InspectorPanel } from '@/modules/spaces/components/inspector-panel';
import { LiveTurn } from '@/modules/spaces/components/live-turn';
import { MessageList } from '@/modules/spaces/components/message-list';
import { PausedBanner } from '@/modules/spaces/components/paused-banner';
import { PrPanel } from '@/modules/spaces/components/pr-panel';
import { ProposalPanel } from '@/modules/spaces/components/proposal-panel';
import { TeamPanel } from '@/modules/spaces/components/team-panel';
import { ThreadPane } from '@/modules/spaces/components/thread-pane';
import { TurnPanel } from '@/modules/spaces/components/turn-panel';
import { useLiveTurn } from '@/modules/spaces/hooks/use-live-turn';
import { useScrollToHash } from '@/modules/spaces/hooks/use-scroll-to-hash';
import { useStickToBottom } from '@/modules/spaces/hooks/use-stick-to-bottom';
import { blocksUser } from '@/modules/spaces/utils/blocks-user';
import { browserAction } from '@/modules/spaces/utils/browser-action';
import { checkpointAnchor, unanchoredCheckpoints } from '@/modules/spaces/utils/checkpoint-anchor';
import { groupChains } from '@/modules/spaces/utils/direct-chains';
import { groupThreads } from '@/modules/spaces/utils/group-threads';
import { latestPullRequest } from '@/modules/spaces/utils/pull-request';
import type { Project, SpaceDetail, WorkItem } from '@/types/api';
import { Button } from '@/components/button';
import { faceOf } from '@/components/creature';

type Props = {
  space: SpaceDetail;
  scope: ReactNode;
  status?: string;
  workItems: WorkItem[];
  projects: Project[];
  diff?: { added: number; removed: number };
  diffPanel?: (onClose: () => void) => ReactNode;
  brief?: string;
  drafts?: ReactNode;
  browserOpen?: boolean;
  onBrowserDriver?: (driver: BrowserDriver | null) => void;
};

type Side =
  | { kind: 'thread'; rootId: number }
  | { kind: 'direct'; chainId: number }
  | { kind: 'turn' }
  | { kind: 'inspector' }
  | { kind: 'diff' }
  | { kind: 'pr' }
  | null;

export function SpaceView({
  space,
  scope,
  status,
  workItems,
  projects,
  diff,
  diffPanel,
  brief,
  drafts,
  browserOpen,
  onBrowserDriver,
}: Props) {
  const messages = useMessages(space.id);
  const failedTurns = useFailedTurns(space.id);
  const direct = useDirectMessages(space.id);
  const turn = useLiveTurn(space.id);
  const wrap = useWrapSpace(space.id);
  const proposal = useProposal(space.id);
  const reopen = useReopenSpace(space.id);
  const closed = space.closedAt !== null;
  const running = turn.status === 'running';
  const [side, setSide] = useState<Side>(null);
  const [teamOpen, setTeamOpen] = useState(false);
  const [attaching, setAttaching] = useState(false);
  const [adding, setAdding] = useState(false);
  const { width, startResize } = useResizableWidth({
    key: 'reef.sidePanel.width',
    initial: 400,
    min: 320,
    maxShare: 0.7,
  });
  const threads = groupThreads(messages.data ?? []);
  const thread =
    side?.kind === 'thread' ? threads.find((t) => t.root.id === side.rootId) : undefined;
  const members = useSpaceMembers(space.id);
  const roles = members.data ?? [];
  const faces = Object.fromEntries(roles.map((r) => [r.botId, r]));
  const workingBotId = turn.botId ?? roles.find((r) => r.lead)?.botId;
  const working = turn.role || space.role || 'agent';
  const look = faceOf(faces, workingBotId);
  const action = browserAction(turn);
  const last = messages.data?.at(-1);
  const { box, onScroll, atBottom, toBottom, hold } = useStickToBottom(
    last?.author === 'user' ? last.id : undefined,
    messages.data?.[0]?.id,
  );
  const chains = groupChains(direct.data ?? [], {
    retryable: failedTurns.data ?? [],
    paused: space.paused !== '',
    running: (running || turn.status === 'done') && turn.directMessageId ? turn.botId : undefined,
  });
  const turns = useSpaceTurns(space.id).data ?? [];
  const rollbackable = useTurns(space.id).data ?? [];
  const anchors = checkpointAnchor(turns, rollbackable, threads);
  const unanchored = unanchoredCheckpoints(rollbackable, anchors);
  const chain = side?.kind === 'direct' ? chains.find((c) => c.id === side.chainId) : undefined;
  const liveDirect =
    running && chain?.messages.some((m) => m.id === turn.directMessageId) ? turn : undefined;
  const turnOpen = side?.kind === 'turn' && turn.status !== 'idle';
  if (side?.kind === 'turn' && turn.status === 'idle') setSide(null);

  useEffect(() => {
    onBrowserDriver?.(action ? { role: working, look, action } : null);
    return () => onBrowserDriver?.(null);
  }, [onBrowserDriver, action, working, look]);

  useEffect(() => {
    if (!teamOpen) return;
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setTeamOpen(false);
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [teamOpen]);

  useScrollToHash(messages.isSuccess && ![messages, failedTurns].some((q) => q.isFetching));

  const closeSide = () => setSide(null);
  const openSide = (next: Side) => {
    setSide(next);
    setTeamOpen(false);
  };
  const wide = side?.kind === 'diff';
  const fullSheet = thread !== undefined || chain !== undefined || wide || turnOpen;
  const showTeam =
    !thread && !chain && side?.kind !== 'inspector' && side?.kind !== 'pr' && !wide && !turnOpen;
  const wrapDisabled =
    running || wrap.isPending || proposal.data?.kind === 'wrap' || blocksUser(space.paused);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <SpaceHeader
        space={space}
        scope={scope}
        status={status}
        roles={roles.map((r) => r.botId)}
        faces={faces}
        working={running ? (workingBotId ?? null) : null}
        pullRequest={latestPullRequest(messages.data ?? [])}
        onPullRequest={() => openSide(side?.kind === 'pr' ? null : { kind: 'pr' })}
        pullRequestOpen={side?.kind === 'pr'}
        diff={diff}
        onDiff={diffPanel ? () => openSide(wide ? null : { kind: 'diff' }) : undefined}
        diffOpen={wide}
        wrapDisabled={wrapDisabled}
        onWrap={() => wrap.mutate()}
        attachDisabled={running}
        onAttach={() => setAttaching(true)}
        onAddBot={() => setAdding(true)}
        onInspector={() => openSide(side?.kind === 'inspector' ? null : { kind: 'inspector' })}
        onTeam={() => {
          setTeamOpen(!teamOpen);
          closeSide();
        }}
      />
      {space.workItemId === null && (
        <Dialog open={attaching} onClose={() => setAttaching(false)} title="Attach to a Work item">
          {attaching && (
            <AttachForm space={space} workItems={workItems} onClose={() => setAttaching(false)} />
          )}
        </Dialog>
      )}
      {space.kind !== 'work_item' && (
        <Dialog open={adding} onClose={() => setAdding(false)} title="Add a Bot">
          {adding && <AddBotForm space={space} onClose={() => setAdding(false)} />}
        </Dialog>
      )}
      <div className="relative flex min-h-0 flex-1">
        <section className="flex min-w-0 flex-1 flex-col">
          <div
            ref={box}
            onScroll={onScroll}
            className="min-h-0 flex-1 overflow-y-auto px-3.5 pt-4 md:px-7"
          >
            <div className="mx-auto max-w-5xl space-y-4">
              {wrap.isError && <p className="text-sm text-danger">{wrap.error.message}</p>}
              {messages.isPending && <p className="text-sm text-muted">Loading...</p>}
              {messages.isError && <p className="text-sm text-danger">{messages.error.message}</p>}
              {messages.hasEarlier && (
                <div className="flex justify-center">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    loading={messages.loadingEarlier}
                    onClick={() => {
                      hold();
                      messages.loadEarlier();
                    }}
                  >
                    Load earlier messages
                  </Button>
                </div>
              )}
              {messages.earlierError && (
                <p className="text-sm text-danger">{messages.earlierError.message}</p>
              )}
              {messages.data && (
                <MessageList
                  spaceId={space.id}
                  threads={threads}
                  chains={chains}
                  faces={faces}
                  openId={thread?.root.id}
                  openChainId={chain?.id}
                  running={running}
                  turns={turns}
                  checkpoints={rollbackable}
                  onOpen={(rootId) => openSide({ kind: 'thread', rootId })}
                  onOpenChain={(chainId) => openSide({ kind: 'direct', chainId })}
                />
              )}
              <LiveTurn
                turn={turn}
                role={working}
                look={look}
                open={turnOpen}
                onOpen={() => openSide(turnOpen ? null : { kind: 'turn' })}
              />
              {turn.note && <p className="text-sm text-muted">{turn.note}</p>}
              <FailedTurnRows
                spaceId={space.id}
                faces={faces}
                running={running}
                paused={space.paused !== ''}
              />
              <CheckpointRows spaceId={space.id} running={running} turns={unanchored} />
              <DeniedToolRows space={space} roles={roles} running={running} />
              <ProposalPanel spaceId={space.id} workItemId={space.workItemId} faces={faces} />
              {drafts}
            </div>
          </div>
          {!atBottom && (
            <div className="relative h-0">
              <Button
                size="icon"
                type="button"
                aria-label="Scroll to latest"
                className="absolute bottom-3 left-1/2 z-10 -translate-x-1/2 bg-main shadow-menu"
                onClick={toBottom}
              >
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <path d="M12 5v14M5 12l7 7 7-7" />
                </svg>
              </Button>
            </div>
          )}
          <div className="shrink-0 px-3.5 pb-5 pt-2 md:px-7">
            <div className="mx-auto max-w-5xl space-y-2">
              {space.paused !== '' && <PausedBanner spaceId={space.id} reason={space.paused} />}
              {space.movedTo !== null ? (
                <div className="flex items-center gap-3 rounded-card border border-hairline px-4 py-3">
                  <span className="min-w-0 flex-1 text-sm text-muted">
                    This conversation moved to a new Space
                  </span>
                  <Link
                    to={`/spaces/${space.movedTo}`}
                    className="text-sm text-fg underline underline-offset-2"
                  >
                    Open the Space
                  </Link>
                </div>
              ) : closed ? (
                <div className="flex items-center gap-3 rounded-card border border-hairline px-4 py-3">
                  <span className="min-w-0 flex-1 text-sm text-muted">Wrapped</span>
                  {reopen.isError && (
                    <span className="text-sm text-danger">{reopen.error.message}</span>
                  )}
                  <Button type="button" disabled={reopen.isPending} onClick={() => reopen.mutate()}>
                    Reopen
                  </Button>
                </div>
              ) : (
                <Composer
                  spaceId={space.id}
                  running={running}
                  paused={blocksUser(space.paused)}
                  reason={space.paused}
                  roles={roles}
                  faces={faces}
                  working={working}
                  workingBotId={workingBotId}
                />
              )}
            </div>
          </div>
        </section>
        {!fullSheet && (side || teamOpen) && (
          <button
            type="button"
            aria-label="Close sheet"
            className={`fixed inset-0 z-10 ${side ? 'bg-scrim/70 md:hidden' : 'max-md:bg-scrim/70'}`}
            onClick={() => {
              closeSide();
              setTeamOpen(false);
            }}
          />
        )}
        {!showTeam && (
          <div
            style={{ '--side-width': `${width}px` } as CSSProperties}
            className={`relative shrink-0 md:w-(--side-width) md:border-l md:border-hairline ${browserOpen ? 'md:max-[1539px]:absolute md:max-[1539px]:inset-y-0 md:max-[1539px]:right-0 md:max-[1539px]:z-20 md:max-[1539px]:bg-main' : 'md:max-[1099px]:absolute md:max-[1099px]:inset-y-0 md:max-[1099px]:right-0 md:max-[1099px]:z-20 md:max-[1099px]:bg-main'} ${fullSheet ? 'max-md:fixed max-md:inset-0 max-md:z-30 max-md:bg-main' : 'max-md:fixed max-md:inset-x-0 max-md:bottom-0 max-md:z-20 max-md:rounded-t-[28px] max-md:bg-field'}`}
          >
            <ResizeHandle
              label="Resize panel"
              onPointerDown={startResize}
              className="max-md:hidden"
            />
            <div
              className={`h-full overflow-y-auto ${fullSheet ? 'flex flex-col' : 'p-4 max-md:max-h-[85dvh] max-md:pb-8'}`}
            >
              {!fullSheet && (
                <div className="mx-auto mb-4 h-[5px] w-[38px] rounded-pill bg-raised md:hidden" />
              )}
              {thread && (
                <ThreadPane
                  key={thread.root.id}
                  spaceId={space.id}
                  thread={thread}
                  lead={space.role}
                  roles={roles}
                  faces={faces}
                  running={running}
                  paused={blocksUser(space.paused)}
                  reason={space.paused}
                  working={working}
                  workingBotId={workingBotId}
                  closed={closed}
                  onClose={closeSide}
                />
              )}
              {chain && (
                <CoordinationPanel
                  key={chain.id}
                  spaceId={space.id}
                  chain={chain}
                  live={liveDirect}
                  faces={faces}
                  running={running}
                  paused={space.paused !== ''}
                  reason={space.paused}
                  onOpenSource={(id) => {
                    const source = messages.data?.find((m) => m.id === id);
                    if (source) openSide({ kind: 'thread', rootId: source.parentId ?? source.id });
                  }}
                  onClose={closeSide}
                />
              )}
              {turnOpen && <TurnPanel turn={turn} role={working} look={look} onClose={closeSide} />}
              {wide && diffPanel?.(closeSide)}
              {side?.kind === 'inspector' && (
                <InspectorPanel spaceId={space.id} faces={faces} onClose={closeSide} />
              )}
              {side?.kind === 'pr' && space.workItemId !== null && (
                <PrPanel workItemId={space.workItemId} onClose={closeSide} />
              )}
            </div>
          </div>
        )}
        {showTeam && teamOpen && (
          <div className="z-20 max-md:fixed max-md:inset-x-0 max-md:bottom-0 max-md:rounded-t-[28px] max-md:bg-field md:absolute md:right-4 md:top-2 md:w-80 md:rounded-card md:border md:border-hairline md:bg-field md:shadow-menu">
            <div className="overflow-y-auto p-4 max-md:max-h-[85dvh] max-md:pb-8 md:max-h-[70dvh]">
              <div className="mx-auto mb-4 h-[5px] w-[38px] rounded-pill bg-raised md:hidden" />
              <div className="mb-5 flex items-center gap-2.5 md:hidden">
                <div className="min-w-0 flex-1">
                  <div className="truncate text-xl">#{space.name}</div>
                  {space.team && <div className="text-[13px] text-muted">Crew · {space.team}</div>}
                </div>
                {space.workItemId !== null && !closed && (
                  <Button
                    variant="primary"
                    type="button"
                    disabled={wrapDisabled}
                    onClick={() => wrap.mutate()}
                  >
                    Wrap up
                  </Button>
                )}
              </div>
              <TeamPanel
                spaceId={space.id}
                workItemId={space.workItemId}
                roles={roles}
                projects={projects}
                working={running ? (workingBotId ?? null) : null}
                brief={brief}
                onClose={() => setTeamOpen(false)}
              />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
