import { describe, expect, it } from 'vitest';
import type { LiveTurn, Tool } from './fold-events';
import { draftText, recentTools, toolCount } from './turn-status';

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

describe('recentTools', () => {
  it('keeps the last n top-level calls in order, skipping text', () => {
    const items: LiveTurn['items'] = [
      { type: 'tool', tool: tool('a', 'Read', {}) },
      { type: 'tool', tool: tool('b', 'Grep', {}) },
      { type: 'text', text: 'x' },
      { type: 'tool', tool: tool('c', 'Bash', {}) },
      { type: 'tool', tool: tool('d', 'Edit', {}) },
    ];
    expect(recentTools(turn(items), 3).map((t) => t.id)).toEqual(['b', 'c', 'd']);
    expect(recentTools(turn(items.slice(0, 1)), 3).map((t) => t.id)).toEqual(['a']);
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

describe('draftText', () => {
  const items: LiveTurn['items'] = [
    { type: 'text', text: 'Looking.' },
    { type: 'tool', tool: tool('a', 'Read', {}) },
    { type: 'text', text: 'Found it.' },
  ];

  it('joins the streamed text of a running or finishing turn', () => {
    expect(draftText(turn(items))).toBe('Looking.\n\nFound it.');
    expect(draftText({ ...turn(items), status: 'done' })).toBe('Looking.\n\nFound it.');
  });

  it('is empty before any text, for a direct turn and for a failed turn', () => {
    expect(draftText(turn([{ type: 'tool', tool: tool('a', 'Read', {}) }]))).toBe('');
    expect(draftText({ ...turn(items), directMessageId: 1 })).toBe('');
    expect(draftText({ ...turn(items), status: 'failed' })).toBe('');
  });
});
