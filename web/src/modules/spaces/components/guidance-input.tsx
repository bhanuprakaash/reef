import { useState } from 'react';
import { useBotActivity } from '@/modules/spaces/api/get-bot-activity';
import { useGuideBot } from '@/modules/spaces/api/guide-bot';
import { useStopSpace } from '@/modules/spaces/api/stop-space';
import { ExchangeActions, exchangeState } from '@/modules/spaces/components/exchange-link';
import { Button } from '@/components/button';
import { TextArea } from '@/components/text-area';

type Props = {
  spaceId: number;
  botId: number;
  botName: string;
  paused: boolean;
  running?: boolean;
};

export function GuidanceInput({ spaceId, botId, botName, paused, running }: Props) {
  const [text, setText] = useState('');
  const [key, setKey] = useState(() => crypto.randomUUID());
  const guide = useGuideBot(spaceId, botId);
  const stop = useStopSpace(spaceId);
  const activity = useBotActivity(spaceId, botId);
  const where = activity.data?.label ?? '';
  const label =
    activity.data?.space.kind === 'consultation'
      ? `Guide ${botName} in ${where}`
      : `Message ${where}`;
  const sent = (activity.data?.exchanges ?? []).filter(
    (d) => d.kind === 'guidance' && d.from === 'user',
  );

  return (
    <div className="space-y-2">
      {sent.length > 0 && (
        <ul className="space-y-1">
          {sent.map((d) => (
            <li key={d.id} className="flex gap-2 text-[13px] text-muted">
              <span className="min-w-0 flex-1 truncate">{d.body}</span>
              <span className="shrink-0">{exchangeState(d, paused)}</span>
              <ExchangeActions request={d} />
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          const body = text.trim();
          if (body === '' || guide.isPending) return;
          guide.mutate(
            { body, key },
            {
              onSuccess: () => {
                setText('');
                setKey(crypto.randomUUID());
              },
            },
          );
        }}
      >
        <TextArea
          aria-label={label}
          placeholder={label}
          rows={1}
          className="min-w-0 flex-1"
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
        {running && (
          <Button
            type="button"
            variant="secondary"
            disabled={stop.isPending}
            onClick={() => stop.mutate()}
          >
            Stop
          </Button>
        )}
        <Button type="submit" variant="primary" disabled={text.trim() === '' || guide.isPending}>
          Send
        </Button>
      </form>
      {guide.isError && <p className="text-sm text-danger">{guide.error.message}</p>}
    </div>
  );
}
