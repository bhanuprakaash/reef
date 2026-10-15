import { useEffect, useState } from 'react';
import { Creature } from '@/components/creature';
import type { LiveTurn as LiveTurnState } from '@/modules/channels/utils/fold-events';
import { liveStatus, toolCount } from '@/modules/channels/utils/turn-status';

type Props = {
  turn: LiveTurnState;
  role: string;
  face?: string;
  open?: boolean;
  onOpen?: () => void;
};

function Elapsed() {
  const [start] = useState(Date.now);
  const [now, setNow] = useState(start);

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  const seconds = Math.floor((now - start) / 1000);
  return <>{seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`}</>;
}

export function LiveTurn({ turn, role, face, open, onOpen }: Props) {
  if (turn.status === 'idle') return null;

  const running = turn.status === 'running';
  const tools = toolCount(turn);

  return (
    <button
      type="button"
      aria-expanded={open}
      onClick={onOpen}
      className="flex w-full items-center gap-3 rounded-card border border-line bg-card px-4 py-3 text-left hover:bg-hover [--creature-ring:var(--color-card)]"
    >
      <Creature role={role} face={face} size={24} state={running ? 'working' : 'failed'} />
      <span className="min-w-0 flex-1">
        <span className="block text-[14.5px] capitalize">
          {role} {running && <span className="normal-case">is working</span>}
        </span>
        {running ? (
          <span className="block truncate text-[13px] text-muted">{liveStatus(turn)}</span>
        ) : (
          <span className="block truncate text-[13px] text-danger">Failed: {turn.error}</span>
        )}
      </span>
      {running && (
        <span className="shrink-0 text-[13px] tabular-nums text-muted">
          <Elapsed key={role} /> · {tools} {tools === 1 ? 'tool' : 'tools'}
        </span>
      )}
      <svg
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        className="shrink-0 text-muted"
      >
        <path d="M9 5l7 7-7 7" />
      </svg>
    </button>
  );
}
