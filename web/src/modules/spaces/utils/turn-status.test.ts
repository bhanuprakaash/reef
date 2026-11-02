import { describe, expect, it } from 'vitest';
import type { LiveTurn, Tool } from './fold-events';
import type { Turn } from '@/types/api';
import { draftText, failedRetryTarget, recentTools, toolCount } from './turn-status';

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
  const post = (id: string, isError = false): LiveTurn['items'][number] => ({
    type: 'tool',
    tool: { ...tool(id, 'mcp__reef__post_to_space', {}), done: true, isError },
  });
  const read: LiveTurn['items'][number] = { type: 'tool', tool: tool('r', 'Read', {}) };
  const items: LiveTurn['items'] = [
    { type: 'text', text: 'Looking.' },
    read,
    { type: 'text', text: 'Found it.' },
  ];

  it('joins the streamed text of a running or finishing turn the way the runner does', () => {
    expect(draftText(turn(items))).toBe('Looking.Found it.');
    expect(draftText({ ...turn(items), status: 'done' })).toBe('Looking.Found it.');
  });

  it('is empty before any text, for a direct turn and for a failed turn', () => {
    expect(draftText(turn([read]))).toBe('');
    expect(draftText({ ...turn(items), directMessageId: 1 })).toBe('');
    expect(draftText({ ...turn(items), status: 'failed' })).toBe('');
  });

  it('keeps only the text after the last post when other work followed it', () => {
    const after: LiveTurn['items'] = [
      ...items,
      post('p'),
      { type: 'text', text: 'Posted.' },
      read,
      { type: 'text', text: ' Tests pass. ' },
    ];
    expect(draftText(turn(after))).toBe('Posted. Tests pass.');
  });

  it('is empty when a post is the last work, even with text after it', () => {
    expect(draftText(turn([...items, post('p'), { type: 'text', text: 'Posted.' }]))).toBe('');
  });

  it('ignores a post that failed or has not returned', () => {
    expect(draftText(turn([...items, post('p', true)]))).toBe('Looking.Found it.');
    const pending: LiveTurn['items'][number] = {
      type: 'tool',
      tool: tool('p', 'mcp__reef__post_to_space', {}),
    };
    expect(draftText(turn([...items, pending]))).toBe('Looking.Found it.');
  });
});

describe('failedRetryTarget', () => {
  const failed = [
    { id: 1, botId: 3 },
    { id: 2, botId: 4 },
    { id: 3, botId: 3 },
  ] as Turn[];

  it('is the latest failed turn of the Bot while the live turn failed', () => {
    expect(failedRetryTarget(failed, { status: 'failed', botId: 3, items: [] })?.id).toBe(3);
    expect(failedRetryTarget(failed, { status: 'running', botId: 3, items: [] })).toBeUndefined();
    expect(failedRetryTarget(failed, { status: 'failed', botId: 9, items: [] })).toBeUndefined();
  });
});
