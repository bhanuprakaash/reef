import type { InspectorToolCall, InspectorTurn } from '@/types/api';

export type Denial = { turnId: number; role: string; pattern: string; needsWrite: boolean };

const _editTools = ['Edit', 'Write', 'MultiEdit', 'NotebookEdit'];
const _notGranted = /^Claude requested permissions to use \S+, but you haven't granted it yet\./;
const _needsApproval = /^This command requires approval|^.+ needs approval\. The path is inside/s;
const _disabled =
  /^(?:<tool_use_error>)?(?:Error: )?No such tool available: (\w+)\. \1 is disabled for this session/;

function bashPattern(input: string): string | null {
  let command: unknown;
  try {
    command = (JSON.parse(input) as { command?: unknown }).command;
  } catch {
    return null;
  }
  if (typeof command !== 'string' || /[;&|<>`$()\n]/.test(command)) return null;
  const word = command.trim().split(/\s+/)[0];
  return /^[\w./-]+$/.test(word) ? `Bash(${word}:*)` : null;
}

function withChildren(call: InspectorToolCall): InspectorToolCall[] {
  return [call, ...(call.children ?? []).flatMap(withChildren)];
}

export function deniedTools(turns: InspectorTurn[]): Denial[] {
  const latest = new Map<string, InspectorTurn>();
  for (const turn of turns) {
    if (turn.id > (latest.get(turn.role)?.id ?? 0)) latest.set(turn.role, turn);
  }
  const found: Denial[] = [];
  for (const turn of latest.values()) {
    if (turn.status === 'running') continue;
    for (const call of turn.toolCalls.flatMap(withChildren)) {
      const disabled = _disabled.exec(call.result)?.[1];
      let pattern: string | null = null;
      if (call.name === 'Bash') {
        if (_notGranted.test(call.result) || _needsApproval.test(call.result)) {
          pattern = bashPattern(call.input);
        }
      } else if (_notGranted.test(call.result) || (disabled && _editTools.includes(disabled))) {
        pattern = call.name;
      }
      if (pattern === null) continue;
      if (found.some((d) => d.turnId === turn.id && d.pattern === pattern)) continue;
      found.push({
        turnId: turn.id,
        role: turn.role,
        pattern,
        needsWrite: disabled !== undefined,
      });
    }
  }
  return found;
}

export function isAllowed(tools: string[], pattern: string): boolean {
  return tools.includes(pattern) || tools.includes(pattern.split('(')[0]);
}
