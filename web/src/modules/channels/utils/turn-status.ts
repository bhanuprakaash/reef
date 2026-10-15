import type { LiveTurn, Tool } from '@/modules/channels/utils/fold-events';
import { summarizeInput } from '@/modules/channels/utils/summarize-input';

function latest(tool: Tool): Tool {
  const child = tool.children.at(-1);
  return child ? latest(child) : tool;
}

function count(tool: Tool): number {
  return 1 + tool.children.reduce((n, c) => n + count(c), 0);
}

export function toolCount(turn: LiveTurn): number {
  return turn.items.reduce((n, i) => (i.type === 'tool' ? n + count(i.tool) : n), 0);
}

export function liveStatus(turn: LiveTurn): string {
  const last = turn.items.at(-1);
  if (!last) return 'starting…';
  if (last.type === 'text') return 'writing…';
  const tool = latest(last.tool);
  return `${tool.name} ${summarizeInput(tool.input)}`.trim();
}
