import { useEffect, useRef, useState, type CSSProperties } from 'react';
import { Creature, type Look } from '@/components/creature';
import { useRetryTurn } from '@/lib/api/retry-turn';
import { useMediaQuery } from '@/lib/use-media-query';
import { useFailedTurns } from '@/modules/spaces/api/get-failed-turns';
import { useRunningTurns } from '@/modules/spaces/api/get-turns';
import { ToolRow } from '@/modules/spaces/components/tool-row';
import { activity } from '@/modules/spaces/utils/activity';
import { dockLayout } from '@/modules/spaces/utils/dock-layout';
import type { LiveTurn } from '@/modules/spaces/utils/fold-events';
import { failedRetryTarget, recentTools, toolCount } from '@/modules/spaces/utils/turn-status';

type Entry = { turn: LiveTurn; role: string; look?: Look };

type Waiting = { botId: number; role: string; look?: Look };

type Props = {
  spaceId: number;
  entries: Entry[];
  waiting?: Waiting[];
  paused: boolean;
  onOpenTurn: () => void;
};

const bubbleSpecs = [
  [0, 6, 3.6, 0],
  [10, 4, 3, -0.9],
  [19, 7, 4, -1.8],
  [27, 5, 3.2, -2.5],
  [5, 5, 3.4, -1.3],
];

function Bubbles({ slow }: { slow: boolean }) {
  return (
    <span className={`dock-bubbles${slow ? ' slow' : ''}`} aria-hidden="true">
      {bubbleSpecs.map(([x, s, t, d]) => (
        <i
          key={x}
          style={
            { '--x': `${x}px`, '--s': `${s}px`, '--t': `${t}s`, '--d': `${d}s` } as CSSProperties
          }
        />
      ))}
    </span>
  );
}

function Elapsed({ startedAt }: { startedAt?: string }) {
  const [now, setNow] = useState(Date.now);

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  if (!startedAt) return null;
  const seconds = Math.max(0, Math.floor((now - Date.parse(startedAt)) / 1000));
  return <>{seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`}</>;
}

function Face({ entry }: { entry: Entry }) {
  const running = entry.turn.status === 'running';
  return (
    <span className="relative shrink-0 [--creature-ring:transparent]">
      {running && <Bubbles slow={Boolean(entry.turn.directMessageId) || !activity(entry.turn)} />}
      <Creature
        look={entry.look}
        size={30}
        state={running ? 'working' : 'failed'}
        animate={running}
      />
    </span>
  );
}

function Status({
  spaceId,
  entry,
  startedAt,
  paused,
}: {
  spaceId: number;
  entry: Entry;
  startedAt?: string;
  paused: boolean;
}) {
  const failed = useFailedTurns(spaceId);
  const retry = useRetryTurn();
  const { turn, role } = entry;
  const name = <span className="shrink-0 font-medium capitalize">{role}</span>;

  if (turn.status === 'running') {
    const now = turn.directMessageId ? null : activity(turn);
    return (
      <>
        {name}
        <span className="shrink-0 text-xs tabular-nums text-muted">
          <Elapsed startedAt={startedAt} />
        </span>
        <span className="tide min-w-0 truncate">
          is working{now ? ` · ${now.verb} ${now.detail}` : ''}
        </span>
      </>
    );
  }

  const target = failedRetryTarget(failed.data ?? [], turn);
  return (
    <>
      {name}
      <span className="min-w-0 truncate text-failed-fg">Failed: {turn.error}</span>
      {target && !paused && (
        <button
          type="button"
          disabled={retry.isPending}
          onClick={() => retry.mutate(target.id)}
          className="relative h-[26px] shrink-0 rounded-pill bg-failed px-3 text-xs font-medium text-main after:absolute after:-inset-y-2 after:inset-x-0 disabled:opacity-60"
        >
          Retry
        </button>
      )}
      {retry.isError && <span className="min-w-0 truncate text-danger">{retry.error.message}</span>}
    </>
  );
}

function Steps({
  entries,
  startedAt,
  onOpenTurn,
}: {
  entries: Entry[];
  startedAt: (entry: Entry) => string | undefined;
  onOpenTurn: () => void;
}) {
  return (
    <div className="absolute bottom-[calc(100%+10px)] left-0 z-20 w-[min(470px,100%)] space-y-3 rounded-card bg-card px-4 py-3.5 text-[13.5px] shadow-menu">
      {entries
        .filter((e) => e.turn.status === 'running')
        .map((entry) => {
          const direct = Boolean(entry.turn.directMessageId);
          const tools = toolCount(entry.turn);
          return (
            <div key={entry.turn.botId ?? entry.role} className="space-y-2">
              <div className="flex items-center gap-2.5">
                <Creature look={entry.look} size={22} state="working" animate />
                <span className="font-medium capitalize">{entry.role}</span>
                <span className="ml-auto text-xs tabular-nums text-muted">
                  <Elapsed startedAt={startedAt(entry)} /> · {tools}{' '}
                  {tools === 1 ? 'tool' : 'tools'}
                </span>
              </div>
              {!direct && (
                <div className="space-y-1">
                  {recentTools(entry.turn, 5).map((tool) => (
                    <ToolRow key={tool.id} tool={tool} />
                  ))}
                </div>
              )}
            </div>
          );
        })}
      <button
        type="button"
        onClick={onOpenTurn}
        className="ml-auto block text-xs text-muted hover:text-fg"
      >
        Open turn ›
      </button>
    </div>
  );
}

function WaitingPills({ waiting }: { waiting: Waiting[] }) {
  return (
    <div className="flex flex-wrap gap-2">
      {waiting.map((w) => (
        <div
          key={w.botId}
          className="flex h-[42px] max-md:h-11 min-w-0 max-w-full items-center gap-2.5 rounded-pill bg-card pl-2 pr-[18px] text-[13.5px]"
        >
          <span className="shrink-0 [--creature-ring:transparent]">
            <Creature look={w.look} size={30} state="needs-you" />
          </span>
          <span className="shrink-0 font-medium capitalize">{w.role}</span>
          <span className="min-w-0 truncate text-muted">waiting on you</span>
        </div>
      ))}
    </div>
  );
}

export function LiveDock({ spaceId, entries, waiting = [], paused, onOpenTurn }: Props) {
  const live = entries.filter((e) => e.turn.status === 'running' || e.turn.status === 'failed');
  const running = useRunningTurns();
  const startedAt = (entry: Entry) =>
    running.data?.find((t) => t.spaceId === spaceId && t.botId === (entry.turn.botId ?? null))
      ?.startedAt;
  const reduceMotion = useMediaQuery('(prefers-reduced-motion: reduce)');
  const [open, setOpen] = useState(false);
  const [picked, setPicked] = useState<number | null>(null);
  const [tick, setTick] = useState(0);
  const box = useRef<HTMLDivElement>(null);
  const tide = dockLayout(live.map((e) => e.turn)) === 'tide';

  useEffect(() => {
    if (!open) return;
    const outside = (e: PointerEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    const escape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('pointerdown', outside);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('keydown', escape);
    };
  }, [open]);

  useEffect(() => {
    if (!tide || reduceMotion || picked !== null) return;
    const id = setInterval(() => setTick((t) => t + 1), 3600);
    return () => clearInterval(id);
  }, [tide, reduceMotion, picked]);

  if (live.length === 0 && waiting.length === 0) return null;
  if (live.length === 0) return <WaitingPills waiting={waiting} />;

  const active = live[(picked ?? tick) % live.length];

  return (
    <div ref={box} className="relative">
      {open && (
        <Steps
          entries={live}
          startedAt={startedAt}
          onOpenTurn={() => {
            setOpen(false);
            onOpenTurn();
          }}
        />
      )}
      {tide ? (
        <div className="flex h-[46px] items-center gap-3.5 rounded-menu bg-card pl-2.5 pr-3.5 text-[13.5px]">
          <span className="flex shrink-0 items-center gap-1.5">
            {live.map((entry, i) => (
              <button
                key={entry.turn.botId ?? entry.role}
                type="button"
                aria-label={entry.role}
                aria-pressed={entry === active}
                onClick={() => setPicked(i)}
                className={`transition-transform motion-reduce:transition-none ${entry === active ? '-translate-y-[3px] scale-110' : 'opacity-50'}`}
              >
                <Face entry={entry} />
              </button>
            ))}
          </span>
          <span className="flex min-w-0 flex-1 items-center gap-2.5">
            <Status
              spaceId={spaceId}
              entry={active}
              startedAt={startedAt(active)}
              paused={paused}
            />
          </span>
          <button
            type="button"
            aria-expanded={open}
            onClick={() => setOpen(!open)}
            className="shrink-0 text-xs text-muted hover:text-fg"
          >
            {live.length} working
          </button>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          {live.map((entry) =>
            entry.turn.status === 'running' ? (
              <button
                key={entry.turn.botId ?? entry.role}
                type="button"
                aria-expanded={open}
                onClick={() => setOpen(!open)}
                className="dock-water flex h-[42px] max-md:h-11 min-w-0 max-w-full items-center gap-2.5 rounded-pill pl-2 pr-[18px] text-left text-[13.5px]"
              >
                <Face entry={entry} />
                <Status
                  spaceId={spaceId}
                  entry={entry}
                  startedAt={startedAt(entry)}
                  paused={paused}
                />
              </button>
            ) : (
              <div
                key={entry.turn.botId ?? entry.role}
                className="flex h-[42px] max-md:h-11 min-w-0 max-w-full items-center gap-2.5 rounded-pill bg-failed-bg pl-2 pr-3 text-[13.5px]"
              >
                <Face entry={entry} />
                <Status
                  spaceId={spaceId}
                  entry={entry}
                  startedAt={startedAt(entry)}
                  paused={paused}
                />
              </div>
            ),
          )}
        </div>
      )}
      {waiting.length > 0 && (
        <div className="mt-2">
          <WaitingPills waiting={waiting} />
        </div>
      )}
    </div>
  );
}
