import { Link, useParams } from 'react-router';
import { useBots } from '@/lib/api/get-bots';
import { useBotActivity } from '@/modules/spaces/api/get-bot-activity';
import { Author } from '@/modules/spaces/components/author';
import { GuidanceInput } from '@/modules/spaces/components/guidance-input';
import { Bubble } from '@/modules/spaces/components/bubble';
import { rowTime } from '@/utils/row-time';
import { summarizeInput } from '@/modules/spaces/utils/summarize-input';
import { BackButton } from '@/components/back-button';
import { buttonClass } from '@/components/button';
import { Card } from '@/components/card';
import { Chip } from '@/components/chip';
import { EmptyState } from '@/components/empty-state';

export function BotActivityPage() {
  const spaceId = Number(useParams().id);
  const botId = Number(useParams().botId);
  const activity = useBotActivity(spaceId, botId);
  const bots = useBots();

  if (activity.isPending) return <p className="text-sm text-muted">Loading...</p>;
  if (activity.isError) return <p className="text-sm text-danger">{activity.error.message}</p>;
  const { space, label, origin, messages, exchanges, turns } = activity.data;
  const look = (id?: number | null) => bots.data?.find((b) => b.id === id);

  return (
    <div className="mx-auto flex w-full max-w-[820px] flex-col gap-6 py-6">
      <header className="flex flex-wrap items-center gap-4">
        <BackButton fallback={`/bots/${botId}`} />
        <div className="min-w-0 flex-1 basis-56">
          <h1 className="text-[32px] font-normal tracking-[-0.02em]">{label}</h1>
          <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
            {space.closedAt && <Chip>closed</Chip>}
            {origin && (
              <Link
                to={`/spaces/${origin.spaceId}`}
                className="text-[13px] text-muted hover:text-fg"
              >
                Asked from {origin.label}
              </Link>
            )}
          </div>
        </div>
        <Link to={`/spaces/${space.id}`} className={buttonClass('secondary')}>
          Open Space
        </Link>
      </header>

      <Card title="Messages">
        {messages.length === 0 && (
          <EmptyState title="No messages" body="Nothing was said here." compact />
        )}
        <div className="flex flex-col items-start gap-2.5">
          {messages.map((m) => (
            <div key={m.id} className="flex w-full flex-col gap-1">
              <Author name={m.author} look={look(m.botId)} time={rowTime(m.createdAt)} />
              <Bubble own={m.author === 'user'}>{m.body}</Bubble>
            </div>
          ))}
        </div>
      </Card>

      <Card title="Requests" flush>
        {exchanges.length === 0 && (
          <div className="px-6 pb-5">
            <EmptyState title="No requests" body="No other Bot asked or answered." compact />
          </div>
        )}
        <ul>
          {exchanges.map((d) => (
            <li key={d.id} className="border-t border-hairline px-6 py-3.5">
              <div className="flex gap-2 text-[13px] text-muted">
                <span className="first-letter:uppercase">
                  {d.from} to {d.to}
                </span>
                <span>· {d.state ?? d.status}</span>
              </div>
              <p className="mt-1 text-sm">{d.body}</p>
            </li>
          ))}
        </ul>
      </Card>

      {!space.closedAt && (
        <GuidanceInput
          spaceId={spaceId}
          botId={botId}
          botName={look(botId)?.name ?? 'Bot'}
          paused={space.paused !== ''}
        />
      )}

      <Card title="Turns" flush>
        {turns.length === 0 && (
          <div className="px-6 pb-5">
            <EmptyState title="No turns" body="This Bot has not run here." compact />
          </div>
        )}
        <ul>
          {turns.map((t) => (
            <li key={t.id} className="border-t border-hairline px-6 py-3.5">
              <div className="flex gap-2 text-[13px] text-muted">
                <span>Turn {t.id}</span>
                <span>· {t.status}</span>
                <span>· {rowTime(t.startedAt)}</span>
              </div>
              {t.error && <p className="mt-1 text-sm text-danger">{t.error}</p>}
              <div className="mt-2 space-y-1 font-mono text-xs text-muted">
                {t.toolCalls.map((c) => (
                  <details key={c.id} className={c.parentId ? 'ml-4' : ''}>
                    <summary className="flex cursor-pointer gap-2.5">
                      {c.name}
                      <span className="min-w-0 flex-1 truncate text-soft">
                        {summarizeInput(parse(c.input))}
                      </span>
                    </summary>
                    <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-field px-2.5 py-2">
                      {c.input}
                      {'\n\n'}
                      {c.result}
                    </pre>
                  </details>
                ))}
              </div>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}

function parse(input: string): unknown {
  try {
    return JSON.parse(input);
  } catch {
    return input;
  }
}
