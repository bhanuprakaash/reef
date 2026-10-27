import type { Status } from '@/components/status-dot';
import type { DirectMessage, Turn } from '@/types/api';

export type ChainState = 'working' | 'paused' | 'failed' | 'stopped' | 'completed';

export const dotFor: Record<ChainState, Status> = {
  working: 'working',
  paused: 'paused',
  failed: 'failed',
  stopped: 'asleep',
  completed: 'done',
};

export type Chain = {
  id: number;
  messages: DirectMessage[];
  state: ChainState;
  retryTurn?: Turn;
};

type Context = {
  failedTurns: Turn[];
  retryable: Turn[];
  paused: boolean;
  running?: string;
};

function stateOf(messages: DirectMessage[], ctx: Context, failed?: Turn): ChainState {
  const last = messages.at(-1)!;
  const request = messages[0];
  if (last.status === 'failed') {
    if (last.body === 'stopped') return 'stopped';
    if (ctx.running === request.to) return 'working';
    return failed?.error === 'interrupted' ? 'stopped' : 'failed';
  }
  if (last.status === 'done') return 'completed';
  if (ctx.paused) return 'paused';
  return ctx.running ? 'working' : 'completed';
}

export function groupChains(direct: DirectMessage[], ctx: Context): Chain[] {
  const byPair = new Map<string, DirectMessage[]>();
  for (const m of direct) {
    const key = [...[m.from, m.to].sort(), m.sourceMessageId].join('|');
    byPair.set(key, [...(byPair.get(key) ?? []), m]);
  }
  return [...byPair.values()].map((messages) => {
    const newest = Math.max(...messages.map((m) => m.chainId));
    const latest = messages.filter((m) => m.chainId === newest);
    const latestIds = new Set(latest.map((m) => m.id));
    const of = (turns: Turn[]) =>
      turns.filter((t) => t.directMessageId && latestIds.has(t.directMessageId));
    return {
      id: messages[0].chainId,
      messages,
      state: stateOf(latest, ctx, of(ctx.failedTurns).at(-1)),
      retryTurn: of(ctx.retryable).at(-1),
    };
  });
}
