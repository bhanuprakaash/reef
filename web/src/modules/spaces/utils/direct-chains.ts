import type { Status } from '@/components/status-dot';
import type { Approval, DirectMessage, Turn } from '@/types/api';

export type ChainState =
  'working' | 'waiting approval' | 'paused' | 'failed' | 'stopped' | 'rejected' | 'completed';

export const dotFor: Record<ChainState, Status> = {
  working: 'working',
  'waiting approval': 'needs-you',
  paused: 'paused',
  failed: 'failed',
  stopped: 'asleep',
  rejected: 'asleep',
  completed: 'done',
};

export type Chain = {
  id: number;
  messages: DirectMessage[];
  state: ChainState;
  approval?: Approval;
  retryTurn?: Turn;
};

type Context = {
  approvals: Approval[];
  failedTurns: Turn[];
  retryable: Turn[];
  paused: boolean;
  running?: string;
};

function stateOf(
  messages: DirectMessage[],
  ctx: Context,
  approval?: Approval,
  failed?: Turn,
): ChainState {
  const last = messages.at(-1)!;
  const request = messages[0];
  if (approval) return 'waiting approval';
  if (last.status === 'failed') {
    if (last.body === 'rejected') return 'rejected';
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
    const ids = new Set(messages.map((m) => m.id));
    const latestIds = new Set(latest.map((m) => m.id));
    const approval = ctx.approvals.find((a) => a.directMessageId && ids.has(a.directMessageId));
    const of = (turns: Turn[]) =>
      turns.filter((t) => t.directMessageId && latestIds.has(t.directMessageId));
    return {
      id: messages[0].chainId,
      messages,
      state: stateOf(latest, ctx, approval, of(ctx.failedTurns).at(-1)),
      approval,
      retryTurn: of(ctx.retryable).at(-1),
    };
  });
}
