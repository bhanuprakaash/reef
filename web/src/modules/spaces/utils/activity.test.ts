import { describe, expect, it } from 'vitest';
import type { Tool } from '@/modules/spaces/utils/fold-events';
import { activity, bubbleIds, toolVerb } from './activity';

const tool: Tool = {
  id: 't1',
  name: 'Read',
  input: { file_path: 'store/schema.go' },
  done: false,
  output: '',
  isError: false,
  children: [],
};

const running = (...tools: Tool[]) => ({
  status: 'running' as const,
  items: tools.map((t) => ({ type: 'tool' as const, tool: t })),
});

describe('toolVerb', () => {
  it('names what each backend is doing', () => {
    expect(toolVerb('Read')).toBe('Reading');
    expect(toolVerb('Grep')).toBe('Searching');
    expect(toolVerb('commandExecution')).toBe('Running');
    expect(toolVerb('fileChange')).toBe('Editing');
    expect(toolVerb('mcp__reef__remember')).toBe('Calling');
    expect(toolVerb('SomethingNew')).toBe('Working');
  });
});

describe('activity', () => {
  it('reads the running tool, not a finished one', () => {
    expect(activity(running(tool))).toEqual({ verb: 'Reading', detail: 'store/schema.go' });
    expect(activity(running({ ...tool, done: true }))).toBeNull();
  });

  it('follows a subagent into its own tool', () => {
    const parent = {
      ...tool,
      name: 'Agent',
      input: { subagent_type: 'explore' },
      children: [tool],
    };
    expect(activity(running(parent))).toEqual({ verb: 'Reading', detail: 'store/schema.go' });
  });

  it('is null while the agent only thinks, so the caller falls back', () => {
    expect(activity({ status: 'running', items: [{ type: 'text', text: 'hi' }] })).toBeNull();
  });
});

describe('bubbleIds', () => {
  it('gives one id per tool call, newest last', () => {
    const tools = ['a', 'b', 'c', 'd'].map((id) => ({ ...tool, id }));
    expect(bubbleIds(running(...tools), 3)).toEqual(['b', 'c', 'd']);
  });
});
