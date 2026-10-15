import type { LiveTurn } from '@/modules/channels/utils/fold-events';

const actions: Record<string, string> = {
  mcp__reef__browser_navigate: 'opening a page',
  mcp__reef__browser_screenshot: 'taking a screenshot',
  mcp__reef__browser_snapshot: 'reading the page',
  mcp__reef__browser_click: 'clicking',
  mcp__reef__browser_type: 'typing',
  mcp__reef__browser_console: 'reading the console',
  mcp__reef__browser_network: 'reading network requests',
};

export function browserAction(turn: LiveTurn): string | null {
  if (turn.status !== 'running') return null;
  for (const item of turn.items) {
    if (item.type === 'tool' && !item.tool.done && actions[item.tool.name]) {
      return actions[item.tool.name];
    }
  }
  return null;
}
