import type { Item, LiveTurn, Tool } from '@/modules/spaces/utils/fold-events';
import type { Turn } from '@/types/api';

function count(tool: Tool): number {
  return 1 + tool.children.reduce((n, c) => n + count(c), 0);
}

export function toolCount(turn: LiveTurn): number {
  return turn.items.reduce((n, i) => (i.type === 'tool' ? n + count(i.tool) : n), 0);
}

export function recentTools(turn: LiveTurn, n: number): Tool[] {
  return turn.items.flatMap((i) => (i.type === 'tool' ? [i.tool] : [])).slice(-n);
}

const postTool = 'mcp__reef__post_to_space';

function text(items: Item[]): string {
  return items.flatMap((i) => (i.type === 'text' ? [i.text] : [])).join('');
}

export function draftText(turn: LiveTurn): string {
  if (turn.directMessageId || (turn.status !== 'running' && turn.status !== 'done')) return '';
  const posted = turn.items.reduce(
    (at, i, n) =>
      i.type === 'tool' && i.tool.name === postTool && i.tool.done && !i.tool.isError ? n : at,
    -1,
  );
  if (posted < 0) return text(turn.items);
  const after = turn.items.slice(posted + 1);
  const worked = after.some((i) => i.type === 'tool' && i.tool.name !== postTool);
  return worked ? text(after).trim() : '';
}

export function failedRetryTarget(failed: Turn[], turn: LiveTurn): Turn | undefined {
  if (turn.status !== 'failed') return undefined;
  return failed.filter((t) => t.botId === (turn.botId ?? null)).at(-1);
}
