import { type ReactNode, useEffect, useRef, useState } from 'react';
import { Dialog } from '@/components/dialog';
import { useApprovals } from '@/modules/channels/api/get-approvals';
import { useFailedTurns } from '@/modules/channels/api/get-failed-turns';
import { useMessages } from '@/lib/api/get-messages';
import { useTeams } from '@/lib/api/get-teams';
import { useWrapChannel } from '@/modules/channels/api/wrap-channel';
import { ApprovalRows } from '@/modules/channels/components/approval-rows';
import { ChannelHeader } from '@/modules/channels/components/channel-header';
import { CheckpointRows } from '@/modules/channels/components/checkpoint-rows';
import { Composer } from '@/modules/channels/components/composer';
import { FailedTurnRows } from '@/modules/channels/components/failed-turn-rows';
import { InspectorPanel } from '@/modules/channels/components/inspector-panel';
import { LiveTurn } from '@/modules/channels/components/live-turn';
import { MessageList } from '@/modules/channels/components/message-list';
import { PausedBanner } from '@/modules/channels/components/paused-banner';
import { PromoteForm } from '@/modules/channels/components/promote-form';
import { ProposalPanel } from '@/modules/channels/components/proposal-panel';
import { TeamPanel } from '@/modules/channels/components/team-panel';
import { ThreadPane } from '@/modules/channels/components/thread-pane';
import { useLiveTurn } from '@/modules/channels/hooks/use-live-turn';
import { useScrollToHash } from '@/modules/channels/hooks/use-scroll-to-hash';
import { blocksUser } from '@/modules/channels/utils/blocks-user';
import { browserAction } from '@/modules/channels/utils/browser-action';
import { groupThreads } from '@/modules/channels/utils/group-threads';
import { latestPullRequest } from '@/modules/channels/utils/pull-request';
import type { Channel, Feature } from '@/types/api';
import { Button } from '@/components/button';

type Props = {
  channel: Channel;
  scope: ReactNode;
  features: Feature[];
  diff?: { added: number; removed: number };
  diffPanel?: (onClose: () => void) => ReactNode;
  brief?: string;
  browserOpen?: boolean;
  onBrowserDriver?: (driver: { role: string; face?: string; action: string } | null) => void;
};

type Side = { kind: 'thread'; rootId: number } | { kind: 'inspector' } | { kind: 'diff' } | null;

export function ChannelView({
  channel,
  scope,
  features,
  diff,
  diffPanel,
  brief,
  browserOpen,
  onBrowserDriver,
}: Props) {
  const messages = useMessages(channel.id);
  const approvals = useApprovals(channel.id);
  const failedTurns = useFailedTurns(channel.id);
  const turn = useLiveTurn(channel.id);
  const wrap = useWrapChannel(channel.id);
  const end = useRef<HTMLDivElement>(null);
  const running = turn.status === 'running';
  const [side, setSide] = useState<Side>(null);
  const [teamOpen, setTeamOpen] = useState(false);
  const [promoting, setPromoting] = useState(false);
  const threads = groupThreads(messages.data ?? []);
  const thread =
    side?.kind === 'thread' ? threads.find((t) => t.root.id === side.rootId) : undefined;
  const teams = useTeams();
  const roles = teams.data?.find((t) => t.name === channel.team)?.roles ?? [];
  const faces = Object.fromEntries(roles.map((r) => [r.name, r.face]));
  const working = turn.role || channel.role || 'agent';
  const face = faces[working];
  const action = browserAction(turn);

  useEffect(() => {
    onBrowserDriver?.(action ? { role: working, face, action } : null);
    return () => onBrowserDriver?.(null);
  }, [onBrowserDriver, action, working, face]);

  useEffect(() => {
    end.current?.scrollIntoView({ block: 'end' });
  }, [messages.data, turn]);

  useScrollToHash(
    messages.isSuccess && ![messages, approvals, failedTurns].some((q) => q.isFetching),
  );

  const closeSide = () => setSide(null);
  const openSide = (next: Side) => {
    setSide(next);
    setTeamOpen(false);
  };
  const wide = side?.kind === 'diff';
  const fullSheet = thread !== undefined || wide;
  const showTeam = !thread && side?.kind !== 'inspector' && !wide;
  const wrapDisabled = running || wrap.isPending || blocksUser(channel.paused);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <ChannelHeader
        channel={channel}
        scope={scope}
        roles={roles.length > 0 ? roles.map((r) => r.name) : [channel.role]}
        faces={faces}
        pullRequest={latestPullRequest(messages.data ?? [])}
        diff={diff}
        onDiff={diffPanel ? () => openSide(wide ? null : { kind: 'diff' }) : undefined}
        diffOpen={wide}
        wrapDisabled={wrapDisabled}
        onWrap={() => wrap.mutate()}
        promoteDisabled={running}
        onPromote={() => setPromoting(true)}
        onInspector={() => openSide(side?.kind === 'inspector' ? null : { kind: 'inspector' })}
        onTeam={() => {
          setTeamOpen(!showTeam || !teamOpen);
          closeSide();
        }}
        browserOpen={browserOpen}
      />
      {channel.featureId === null && (
        <Dialog open={promoting} onClose={() => setPromoting(false)} title="Promote to a feature">
          {promoting && (
            <PromoteForm
              channel={channel}
              features={features}
              onClose={() => setPromoting(false)}
            />
          )}
        </Dialog>
      )}
      <div className="relative flex min-h-0 flex-1">
        <section className="flex min-w-0 flex-1 flex-col">
          <div className="min-h-0 flex-1 overflow-y-auto px-3.5 pt-4 md:px-7">
            <div className="mx-auto max-w-3xl space-y-4">
              {wrap.isError && <p className="text-sm text-danger">{wrap.error.message}</p>}
              {channel.paused !== '' && (
                <PausedBanner channelId={channel.id} reason={channel.paused} />
              )}
              {messages.isPending && <p className="text-sm text-muted">Loading...</p>}
              {messages.isError && <p className="text-sm text-danger">{messages.error.message}</p>}
              {messages.data && (
                <MessageList
                  threads={threads}
                  faces={faces}
                  openId={thread?.root.id}
                  onOpen={(rootId) => openSide({ kind: 'thread', rootId })}
                />
              )}
              <LiveTurn turn={turn} role={working} face={face} />
              {turn.note && <p className="text-sm text-muted">{turn.note}</p>}
              <FailedTurnRows
                channelId={channel.id}
                faces={faces}
                running={running}
                paused={channel.paused !== ''}
              />
              <CheckpointRows channelId={channel.id} running={running} />
              <ApprovalRows channelId={channel.id} faces={faces} />
              <ProposalPanel channelId={channel.id} featureId={channel.featureId} faces={faces} />
              <div ref={end} />
            </div>
          </div>
          <div className="shrink-0 px-3.5 pb-5 pt-2 md:px-7">
            <div className="mx-auto max-w-3xl">
              <Composer
                channelId={channel.id}
                running={running}
                paused={blocksUser(channel.paused)}
                reason={channel.paused}
                roles={roles.map((r) => r.name)}
                faces={faces}
                working={working}
              />
            </div>
          </div>
        </section>
        {!fullSheet && (side || teamOpen) && (
          <button
            type="button"
            aria-label="Close sheet"
            className="fixed inset-0 z-10 bg-scrim/70 md:hidden"
            onClick={() => {
              closeSide();
              setTeamOpen(false);
            }}
          />
        )}
        <div
          className={`shrink-0 overflow-y-auto ${wide ? 'md:w-[min(840px,65%)]' : 'md:w-[400px]'} md:border-l md:border-hairline ${browserOpen ? 'md:max-[1539px]:absolute md:max-[1539px]:inset-y-0 md:max-[1539px]:right-0 md:max-[1539px]:z-20 md:max-[1539px]:bg-main' : 'md:max-[1099px]:absolute md:max-[1099px]:inset-y-0 md:max-[1099px]:right-0 md:max-[1099px]:z-20 md:max-[1099px]:bg-main'} ${fullSheet ? 'flex flex-col max-md:fixed max-md:inset-0 max-md:z-30 max-md:bg-main' : 'p-4 max-md:fixed max-md:inset-x-0 max-md:bottom-0 max-md:z-20 max-md:max-h-[85dvh] max-md:rounded-t-[28px] max-md:bg-field max-md:pb-8'} ${showTeam && !teamOpen ? (browserOpen ? 'hidden min-[1540px]:block' : 'hidden min-[1100px]:block') : ''}`}
        >
          {!fullSheet && (
            <div className="mx-auto mb-4 h-[5px] w-[38px] rounded-pill bg-raised md:hidden" />
          )}
          {thread && (
            <ThreadPane
              key={thread.root.id}
              channelId={channel.id}
              thread={thread}
              lead={channel.role}
              roles={roles.map((r) => r.name)}
              faces={faces}
              running={running}
              paused={blocksUser(channel.paused)}
              reason={channel.paused}
              working={working}
              onClose={closeSide}
            />
          )}
          {wide && diffPanel?.(closeSide)}
          {side?.kind === 'inspector' && (
            <InspectorPanel channelId={channel.id} faces={faces} onClose={closeSide} />
          )}
          {showTeam && (
            <div className="mb-5 flex items-center gap-2.5 md:hidden">
              <div className="min-w-0 flex-1">
                <div className="truncate text-xl">#{channel.name}</div>
                <div className="text-[13px] text-muted">Team · {channel.team}</div>
              </div>
              {channel.featureId !== null && (
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
          )}
          {showTeam && (
            <TeamPanel
              channelId={channel.id}
              featureId={channel.featureId}
              roles={roles}
              working={running ? working : null}
              waiting={approvals.data?.map((a) => a.to) ?? []}
              brief={brief}
            />
          )}
        </div>
      </div>
    </div>
  );
}
