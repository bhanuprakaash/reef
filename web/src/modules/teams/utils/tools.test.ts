import { describe, expect, it } from 'vitest';
import { builtinTools, customTools, mcpTool, toggleTool } from './tools';

describe('tools', () => {
  it('lists the built-in tools in a fixed order', () => {
    expect(builtinTools).toEqual([
      'Read',
      'Grep',
      'Glob',
      'Edit',
      'Write',
      'Bash',
      'WebFetch',
      'WebSearch',
    ]);
  });

  it('names an MCP server entry the way claude allows it', () => {
    expect(mcpTool('mcp1')).toBe('mcp__mcp1__*');
  });

  it('adds a tool once and keeps the others in place', () => {
    expect(toggleTool(['Read', 'Bash(git log:*)'], 'Edit', true)).toEqual([
      'Read',
      'Bash(git log:*)',
      'Edit',
    ]);
    expect(toggleTool(['Read'], 'Read', true)).toEqual(['Read']);
  });

  it('removes a tool and keeps the others in place', () => {
    expect(toggleTool(['Read', 'Bash(git log:*)', 'Edit'], 'Read', false)).toEqual([
      'Bash(git log:*)',
      'Edit',
    ]);
  });

  it('keeps in the custom list what no checkbox shows', () => {
    expect(
      customTools(
        ['Read', 'mcp__mcp1__*', 'Bash(git log:*)', 'mcp__other__*', 'Skill(how)'],
        ['mcp1'],
      ),
    ).toEqual(['Bash(git log:*)', 'mcp__other__*', 'Skill(how)']);
  });
});
