import type { LiveTurn, Tool } from '@/modules/channels/utils/fold-events';

function count(tool: Tool): number {
  return 1 + tool.children.reduce((n, c) => n + count(c), 0);
}

export function toolCount(turn: LiveTurn): number {
  return turn.items.reduce((n, i) => (i.type === 'tool' ? n + count(i.tool) : n), 0);
}

export function recentTools(turn: LiveTurn, n: number): Tool[] {
  return turn.items.flatMap((i) => (i.type === 'tool' ? [i.tool] : [])).slice(-n);
}
