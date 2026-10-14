import { useEffect, useRef, useState } from 'react';
import { useApprovals } from '@/modules/channels/api/get-approvals';
import { useFailedTurns } from '@/modules/channels/api/get-failed-turns';
import { useMessages } from '@/modules/channels/api/get-messages';
import { useWrapChannel } from '@/modules/channels/api/wrap-channel';
import { ApprovalRows } from '@/modules/channels/components/approval-rows';
import { CheckpointRows } from '@/modules/channels/components/checkpoint-rows';
import { Composer } from '@/modules/channels/components/composer';
import { FailedTurnRows } from '@/modules/channels/components/failed-turn-rows';
import { LiveTurn } from '@/modules/channels/components/live-turn';
import { MessageList } from '@/modules/channels/components/message-list';
import { PausedBanner } from '@/modules/channels/components/paused-banner';
import { PromoteForm } from '@/modules/channels/components/promote-form';
import { ProposalPanel } from '@/modules/channels/components/proposal-panel';
import { ThreadPane } from '@/modules/channels/components/thread-pane';
import { useLiveTurn } from '@/modules/channels/hooks/use-live-turn';
import { useScrollToHash } from '@/modules/channels/hooks/use-scroll-to-hash';
import { blocksUser } from '@/modules/channels/utils/blocks-user';
import { groupThreads } from '@/modules/channels/utils/group-threads';
import type { Channel, Feature } from '@/types/api';
import { buttonClass, errorClass, mutedClass } from '@/utils/classes';

type Props = { channel: Channel; scope: string; features: Feature[] };

export function ChannelView({ channel, scope, features }: Props) {
  const messages = useMessages(channel.id);
  const approvals = useApprovals(channel.id);
  const failedTurns = useFailedTurns(channel.id);
  const turn = useLiveTurn(channel.id);
  const wrap = useWrapChannel(channel.id);
  const end = useRef<HTMLDivElement>(null);
  const running = turn.status === 'running';
  const [threadId, setThreadId] = useState<number | null>(null);
  const threads = groupThreads(messages.data ?? []);
  const thread = threads.find((t) => t.root.id === threadId);

  useEffect(() => {
    end.current?.scrollIntoView({ block: 'end' });
  }, [messages.data, turn]);

  useScrollToHash(
    messages.isSuccess && ![messages, approvals, failedTurns].some((q) => q.isFetching),
  );

  return (
    <div className="flex items-start gap-6">
      <div className="min-w-0 max-w-3xl flex-1 space-y-4">
        <header className="flex items-start justify-between gap-3">
          <div className="space-y-1">
            <h1 className="text-xl font-semibold">{channel.name}</h1>
            <p className={mutedClass}>
              {channel.backend || 'no backend'}
              {channel.permissionMode && ` (${channel.permissionMode})`} · {scope}
            </p>
          </div>
          {channel.featureId !== null && (
            <button
              type="button"
              className={buttonClass}
              disabled={running || wrap.isPending || blocksUser(channel.paused)}
              onClick={() => wrap.mutate()}
            >
              Wrap up
            </button>
          )}
        </header>
        {channel.featureId === null && (
          <PromoteForm channel={channel} features={features} disabled={running} />
        )}
        {wrap.isError && <p className={errorClass}>{wrap.error.message}</p>}
        {channel.paused !== '' && <PausedBanner channelId={channel.id} reason={channel.paused} />}
        {messages.isPending && <p className={mutedClass}>Loading...</p>}
        {messages.isError && <p className={errorClass}>{messages.error.message}</p>}
        {messages.data && <MessageList threads={threads} onOpen={setThreadId} />}
        <LiveTurn turn={turn} role={turn.role || channel.role || 'agent'} />
        {turn.note && <p className={mutedClass}>{turn.note}</p>}
        <FailedTurnRows channelId={channel.id} running={running} paused={channel.paused !== ''} />
        <CheckpointRows channelId={channel.id} running={running} />
        <ApprovalRows channelId={channel.id} />
        <ProposalPanel channelId={channel.id} featureId={channel.featureId} />
        <div className="sticky bottom-0 bg-white pb-1 pt-2 dark:bg-neutral-900">
          <Composer channelId={channel.id} running={running} paused={blocksUser(channel.paused)} />
        </div>
        <div ref={end} />
      </div>
      {thread && (
        <ThreadPane
          key={thread.root.id}
          channelId={channel.id}
          thread={thread}
          running={running}
          paused={blocksUser(channel.paused)}
          onClose={() => setThreadId(null)}
        />
      )}
    </div>
  );
}
