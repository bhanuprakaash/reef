import { describe, expect, it } from 'vitest';
import { deniedTools, isAllowed } from './denied-tools';
import type { InspectorTurn } from '@/types/api';

function turn(
  id: number,
  botId: number | null,
  calls: [string, string, string][],
  status = 'done',
) {
  return {
    id,
    role: `bot-${botId}`,
    botId,
    status,
    toolCalls: calls.map(([name, input, result]) => ({ name, input, result })),
  } as InspectorTurn;
}

const notGranted = "Claude requested permissions to use WebFetch, but you haven't granted it yet.";
const disabled =
  '<tool_use_error>Error: No such tool available: Write. Write is disabled for this session, in subagents as well as here.</tool_use_error>';

describe('deniedTools', () => {
  it('finds a tool the role was not granted', () => {
    expect(deniedTools([turn(1, 1, [['WebFetch', '{}', notGranted]])])).toEqual([
      { turnId: 1, botId: 1, pattern: 'WebFetch', needsWrite: false },
    ]);
  });

  it('finds a tool a subagent was refused, nested under its Agent call', () => {
    const nested = {
      id: 1,
      role: 'lead',
      botId: 1,
      status: 'done',
      toolCalls: [
        {
          name: 'Agent',
          input: '{}',
          result: 'done',
          children: [{ name: 'WebFetch', input: '{}', result: notGranted }],
        },
      ],
    } as InspectorTurn;
    expect(deniedTools([nested])).toEqual([
      { turnId: 1, botId: 1, pattern: 'WebFetch', needsWrite: false },
    ]);
  });

  it('derives a Bash pattern from the command word', () => {
    const calls: [string, string, string][] = [
      ['Bash', '{"command":"npm --version"}', 'This command requires approval'],
      [
        'Bash',
        '{"command":"mkdir out"}',
        "mkdir in '/w/out' needs approval. The path is inside the working directories for this session ('/w'), and Claude Code asks first.",
      ],
    ];
    expect(deniedTools([turn(1, 3, calls)]).map((d) => d.pattern)).toEqual([
      'Bash(npm:*)',
      'Bash(mkdir:*)',
    ]);
  });

  it('offers nothing for a compound Bash command', () => {
    const calls: [string, string, string][] = [
      ['Bash', '{"command":"mkdir a && git status"}', 'This command requires approval'],
    ];
    expect(deniedTools([turn(1, 3, calls)])).toEqual([]);
  });

  it('marks a disabled edit tool as needing write access', () => {
    expect(deniedTools([turn(1, 2, [['Write', '{}', disabled]])])).toEqual([
      { turnId: 1, botId: 2, pattern: 'Write', needsWrite: true },
    ]);
  });

  it('ignores a blocked redirect, a disabled skill and a result that only mentions approval', () => {
    const calls: [string, string, string][] = [
      ['Bash', '{"command":"echo a > b"}', "Output redirection to 'b' was blocked."],
      ['Skill', '{}', 'Error: No such tool available: Skill. Skill is disabled for this session'],
      ['Read', '{}', 'docs: This command requires approval'],
    ];
    expect(deniedTools([turn(1, 3, calls)])).toEqual([]);
  });

  it('keeps only each Bot newest turn, once the turn has ended', () => {
    const turns = [
      turn(1, 1, [['WebFetch', '{}', notGranted]]),
      turn(2, 1, []),
      turn(3, 2, [['WebFetch', '{}', notGranted]], 'running'),
      turn(4, 3, [
        ['WebFetch', '{}', notGranted],
        ['WebFetch', '{}', notGranted],
      ]),
    ];
    expect(deniedTools(turns).map((d) => `${d.botId}:${d.pattern}`)).toEqual(['3:WebFetch']);
  });
});

describe('isAllowed', () => {
  it('matches the pattern or its bare tool', () => {
    expect(isAllowed(['Bash(npm:*)'], 'Bash(npm:*)')).toBe(true);
    expect(isAllowed(['Bash'], 'Bash(npm:*)')).toBe(true);
    expect(isAllowed(['Read'], 'WebFetch')).toBe(false);
  });
});
