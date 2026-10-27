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
  retryable: Turn[];
  paused: boolean;
  running?: string;
};

function stateOf(request: DirectMessage, ctx: Context): ChainState {
  if (request.state === 'stopped') return 'stopped';
  if (request.state === 'failed') return ctx.running === request.to ? 'working' : 'failed';
  if (ctx.paused) return 'paused';
  return request.state === 'answered' ? 'completed' : 'working';
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
      state: stateOf(latest[0], ctx),
      retryTurn: of(ctx.retryable).at(-1),
    };
  });
}
