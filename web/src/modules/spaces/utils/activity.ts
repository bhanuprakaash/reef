import type { LiveTurn, Tool } from '@/modules/spaces/utils/fold-events';
import { subagentLabel } from '@/modules/spaces/utils/subagent-label';
import { summarizeInput } from '@/modules/spaces/utils/summarize-input';

const verbs: Record<string, string> = {
  Read: 'Reading',
  NotebookRead: 'Reading',
  Edit: 'Editing',
  MultiEdit: 'Editing',
  NotebookEdit: 'Editing',
  fileChange: 'Editing',
  Write: 'Writing',
  Grep: 'Searching',
  Glob: 'Searching',
  Bash: 'Running',
  BashOutput: 'Running',
  commandExecution: 'Running',
  Agent: 'Delegating',
  Task: 'Delegating',
  WebFetch: 'Fetching',
  WebSearch: 'Searching',
  TodoWrite: 'Planning',
};

export function toolVerb(name: string): string {
  return verbs[name] ?? (name.startsWith('mcp__') ? 'Calling' : 'Working');
}

export type Activity = { verb: string; detail: string };

function flatten(tools: Tool[]): Tool[] {
  return tools.flatMap((tool) => [tool, ...flatten(tool.children)]);
}

/** The tool the turn is inside right now, or null while the agent only thinks. */
export function activity(turn: LiveTurn): Activity | null {
  const tools = flatten(turn.items.flatMap((i) => (i.type === 'tool' ? [i.tool] : [])));
  const live = tools.filter((tool) => !tool.done).at(-1);
  if (!live) return null;
  return {
    verb: toolVerb(live.name),
    detail: subagentLabel(live.name, live.input) ?? summarizeInput(live.input),
  };
}
