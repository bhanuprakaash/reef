import { describe, expect, it } from 'vitest';
import type { LiveTurn, Tool } from './fold-events';
import { liveStatus, toolCount } from './turn-status';

const tool = (id: string, name: string, input: unknown, children: Tool[] = []): Tool => ({
  id,
  name,
  input,
  done: false,
  output: '',
  isError: false,
  children,
});
const turn = (items: LiveTurn['items']): LiveTurn => ({ status: 'running', items });

describe('liveStatus', () => {
  it('shows the latest tool call, down into its sub-agent calls', () => {
    const agent = tool('a', 'Agent', { prompt: 'p' }, [tool('b', 'Bash', { command: 'npm test' })]);
    expect(liveStatus(turn([{ type: 'tool', tool: agent }]))).toBe('Bash npm test');
  });

  it('says writing while text streams and starting before any event', () => {
    expect(liveStatus(turn([{ type: 'text', text: 'Hi' }]))).toBe('writing…');
    expect(liveStatus(turn([]))).toBe('starting…');
  });
});

describe('toolCount', () => {
  it('counts calls at every depth', () => {
    const agent = tool('a', 'Agent', {}, [tool('b', 'Bash', {})]);
    expect(
      toolCount(
        turn([
          { type: 'tool', tool: agent },
          { type: 'text', text: 'x' },
          { type: 'tool', tool: tool('c', 'Read', {}) },
        ]),
      ),
    ).toBe(3);
  });
});
