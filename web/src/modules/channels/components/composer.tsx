import { useState } from 'react';
import { useSendMessage } from '@/modules/channels/api/send-message';
import { useStopChannel } from '@/modules/channels/api/stop-channel';
import { Button } from '@/components/button';
import { TextArea } from '@/components/text-area';

type Props = { channelId: number; running: boolean; paused: boolean; parent?: number };

export function Composer({ channelId, running, paused, parent }: Props) {
  const [text, setText] = useState('');
  const send = useSendMessage(channelId, parent);
  const stop = useStopChannel(channelId);

  function submit() {
    const trimmed = text.trim();
    if (trimmed === '' || running || paused || send.isPending) return;
    send.mutate(trimmed, { onSuccess: () => setText('') });
  }

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      {send.isError && !paused && <p className="text-sm text-danger">{send.error.message}</p>}
      <TextArea
        className="w-full"
        rows={3}
        placeholder="Message (Enter sends, Shift+Enter adds a line)"
        aria-label={parent === undefined ? 'Message' : 'Reply'}
        disabled={paused}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
            e.preventDefault();
            submit();
          }
        }}
      />
      <div className="flex gap-2">
        <Button variant="primary" type="submit" disabled={running || paused || send.isPending}>
          Send
        </Button>
        {running && (
          <Button type="button" disabled={stop.isPending} onClick={() => stop.mutate()}>
            Stop
          </Button>
        )}
      </div>
    </form>
  );
}
