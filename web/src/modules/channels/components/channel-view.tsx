import { useEffect, useRef } from 'react';
import { useMessages } from '@/modules/channels/api/get-messages';
import { Composer } from '@/modules/channels/components/composer';
import { LiveTurn } from '@/modules/channels/components/live-turn';
import { MessageList } from '@/modules/channels/components/message-list';
import { useLiveTurn } from '@/modules/channels/hooks/use-live-turn';
import type { Channel } from '@/types/api';
import { errorClass, mutedClass } from '@/utils/classes';

export function ChannelView({ channel, scope }: { channel: Channel; scope: string }) {
  const messages = useMessages(channel.id);
  const turn = useLiveTurn(channel.id);
  const end = useRef<HTMLDivElement>(null);

  useEffect(() => {
    end.current?.scrollIntoView({ block: 'end' });
  }, [messages.data, turn]);

  return (
    <div className="max-w-3xl space-y-4">
      <header className="space-y-1">
        <h1 className="text-xl font-semibold">{channel.name}</h1>
        <p className={mutedClass}>
          {channel.backend || 'no backend'}
          {channel.permissionMode && ` (${channel.permissionMode})`} · {scope}
        </p>
      </header>
      {messages.isPending && <p className={mutedClass}>Loading...</p>}
      {messages.isError && <p className={errorClass}>{messages.error.message}</p>}
      {messages.data && <MessageList messages={messages.data} />}
      <LiveTurn turn={turn} role={channel.role || 'agent'} />
      <div ref={end} />
      <div className="sticky bottom-0 bg-white pb-1 pt-2 dark:bg-neutral-900">
        <Composer channelId={channel.id} running={turn.status === 'running'} />
      </div>
    </div>
  );
}
