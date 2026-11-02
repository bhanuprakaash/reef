import type { LiveTurn } from '@/modules/spaces/utils/fold-events';

/** One pill per Bot, until three or more work at once: then one slim tide row. */
export function dockLayout(turns: Pick<LiveTurn, 'status'>[]): 'pills' | 'tide' {
  return turns.filter((t) => t.status === 'running').length >= 3 ? 'tide' : 'pills';
}
