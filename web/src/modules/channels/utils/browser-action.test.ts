import { describe, expect, it } from 'vitest';
import { browserAction } from './browser-action';
import type { LiveTurn, Tool } from './fold-events';

function tool(name: string, done: boolean): Tool {
  return { id: name, name, input: {}, done, output: '', isError: false, children: [] };
}

function turn(status: LiveTurn['status'], ...tools: Tool[]): LiveTurn {
  return { status, role: 'tester', items: tools.map((t) => ({ type: 'tool', tool: t })) };
}

describe('browserAction', () => {
  it('names the browser tool call that is running', () => {
    expect(browserAction(turn('running', tool('mcp__reef__browser_click', false)))).toBe(
      'clicking',
    );
  });

  it('ignores a finished browser call and other tools', () => {
    expect(browserAction(turn('running', tool('mcp__reef__browser_click', true)))).toBeNull();
    expect(browserAction(turn('running', tool('Bash', false)))).toBeNull();
  });

  it('is null once the turn is no longer running', () => {
    expect(browserAction(turn('failed', tool('mcp__reef__browser_click', false)))).toBeNull();
  });
});
