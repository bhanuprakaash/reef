import { useState } from 'react';
import { useSendMessage } from '@/modules/channels/api/send-message';
import { useStopChannel } from '@/modules/channels/api/stop-channel';
import { buttonClass, errorClass, inputClass } from '@/utils/classes';

type Props = { channelId: number; running: boolean; paused: boolean };

export function Composer({ channelId, running, paused }: Props) {
  const [text, setText] = useState('');
  const send = useSendMessage(channelId);
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
      {send.isError && !paused && <p className={errorClass}>{send.error.message}</p>}
      <textarea
        className={`${inputClass} w-full`}
        rows={3}
        placeholder="Message (Enter sends, Shift+Enter adds a line)"
        aria-label="Message"
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
        <button
          type="submit"
          className={buttonClass}
          disabled={running || paused || send.isPending}
        >
          Send
        </button>
        {running && (
          <button
            type="button"
            className={buttonClass}
            disabled={stop.isPending}
            onClick={() => stop.mutate()}
          >
            Stop
          </button>
        )}
      </div>
    </form>
  );
}
