import { describe, expect, it } from 'vitest';
import { emptyTurn, foldEvent, type AgentEvent, type StreamEvent } from './fold-events';

const agent = (event: AgentEvent): StreamEvent => ({ type: 'agent', event });
const fold = (events: StreamEvent[]) => events.reduce(foldEvent, emptyTurn);

describe('foldEvent', () => {
  it('joins text deltas and starts a new text after a tool', () => {
    const got = fold([
      { type: 'turn', status: 'running' },
      agent({ kind: 'text', text: 'Hel' }),
      agent({ kind: 'text', text: 'lo' }),
      agent({ kind: 'tool_call', toolId: 't1', toolName: 'Bash', input: { command: 'ls' } }),
      agent({ kind: 'text', text: 'Done' }),
    ]);
    expect(got).toEqual({
      status: 'running',
      items: [
        { type: 'text', text: 'Hello' },
        {
          type: 'tool',
          tool: {
            id: 't1',
            name: 'Bash',
            input: { command: 'ls' },
            done: false,
            output: '',
            isError: false,
            children: [],
          },
        },
        { type: 'text', text: 'Done' },
      ],
    });
  });

  it('pairs a tool result with its call by tool id', () => {
    const got = fold([
      agent({ kind: 'tool_call', toolId: 't1', toolName: 'Read', input: { file_path: 'a' } }),
      agent({ kind: 'tool_call', toolId: 't2', toolName: 'Bash', input: { command: 'x' } }),
      agent({ kind: 'tool_result', toolId: 't2', output: 'boom', isError: true }),
      agent({ kind: 'tool_result', toolId: 't1', output: 'ok' }),
    ]);
    expect(got.items).toEqual([
      {
        type: 'tool',
        tool: {
          id: 't1',
          name: 'Read',
          input: { file_path: 'a' },
          done: true,
          output: 'ok',
          isError: false,
          children: [],
        },
      },
      {
        type: 'tool',
        tool: {
          id: 't2',
          name: 'Bash',
          input: { command: 'x' },
          done: true,
          output: 'boom',
          isError: true,
          children: [],
        },
      },
    ]);
  });

  it('nests calls with a parent tool id under their Agent call', () => {
    const got = fold([
      agent({ kind: 'tool_call', toolId: 'a1', toolName: 'Agent', input: { prompt: 'p' } }),
      agent({
        kind: 'tool_call',
        toolId: 'b1',
        toolName: 'Bash',
        parentToolId: 'a1',
        input: { command: 'ls' },
      }),
      agent({ kind: 'tool_result', toolId: 'b1', parentToolId: 'a1', output: 'f' }),
    ]);
    expect(got.items).toEqual([
      {
        type: 'tool',
        tool: {
          id: 'a1',
          name: 'Agent',
          input: { prompt: 'p' },
          done: false,
          output: '',
          isError: false,
          children: [
            {
              id: 'b1',
              name: 'Bash',
              input: { command: 'ls' },
              done: true,
              output: 'f',
              isError: false,
              children: [],
            },
          ],
        },
      },
    ]);
  });

  it('keeps the live items when a turn fails and resets on the next running', () => {
    const failed = fold([
      { type: 'turn', status: 'running' },
      agent({ kind: 'text', text: 'partial' }),
      { type: 'turn', status: 'failed', error: 'interrupted' },
    ]);
    expect(failed).toEqual({
      status: 'failed',
      error: 'interrupted',
      items: [{ type: 'text', text: 'partial' }],
    });
    expect(foldEvent(failed, { type: 'turn', status: 'running' })).toEqual({
      status: 'running',
      items: [],
    });
  });

  it('resets to an empty turn on idle, as sent when a subscriber connects between turns', () => {
    const got = fold([
      { type: 'turn', status: 'running' },
      agent({ kind: 'text', text: 'partial' }),
      { type: 'turn', status: 'idle' },
    ]);
    expect(got).toEqual({ status: 'idle', items: [] });
  });

  it('follows the role of each turn in a chain', () => {
    const lead = fold([
      { type: 'turn', status: 'running', role: 'lead' },
      agent({ kind: 'text', text: 'asking' }),
      { type: 'turn', status: 'done', role: 'lead' },
    ]);
    expect(lead.role).toBe('lead');
    expect(foldEvent(lead, { type: 'turn', status: 'running', role: 'architect' })).toEqual({
      status: 'running',
      role: 'architect',
      items: [],
    });
  });

  it('keeps the note of a done turn', () => {
    const got = fold([
      { type: 'turn', status: 'running' },
      { type: 'turn', status: 'done', note: 'no proposal made' },
    ]);
    expect(got).toEqual({ status: 'done', note: 'no proposal made', items: [] });
  });

  it('marks the turn done', () => {
    const got = fold([
      { type: 'turn', status: 'running' },
      agent({ kind: 'text', text: 'hi' }),
      agent({ kind: 'turn_end' }),
      { type: 'turn', status: 'done' },
    ]);
    expect(got.status).toBe('done');
    expect(got.error).toBeUndefined();
  });
});
