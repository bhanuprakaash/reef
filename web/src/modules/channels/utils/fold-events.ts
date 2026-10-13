export type AgentEvent = {
  kind: 'session' | 'text' | 'tool_call' | 'tool_result' | 'usage' | 'turn_end' | 'error';
  text?: string;
  toolId?: string;
  toolName?: string;
  parentToolId?: string;
  input?: unknown;
  output?: string;
  isError?: boolean;
};

export type StreamEvent =
  | { type: 'turn'; status: 'idle' | 'running' | 'done' | 'failed'; error?: string; note?: string }
  | { type: 'agent'; event: AgentEvent };

export type Tool = {
  id: string;
  name: string;
  input: unknown;
  done: boolean;
  output: string;
  isError: boolean;
  children: Tool[];
};

export type Item = { type: 'text'; text: string } | { type: 'tool'; tool: Tool };

export type LiveTurn = {
  status: 'idle' | 'running' | 'done' | 'failed';
  error?: string;
  note?: string;
  items: Item[];
};

export const emptyTurn: LiveTurn = { status: 'idle', items: [] };

function hasTool(tool: Tool, id: string): boolean {
  return tool.id === id || tool.children.some((c) => hasTool(c, id));
}

function mapTool(tool: Tool, id: string, fn: (t: Tool) => Tool): Tool {
  if (tool.id === id) return fn(tool);
  return { ...tool, children: tool.children.map((c) => mapTool(c, id, fn)) };
}

function mapItems(items: Item[], id: string, fn: (t: Tool) => Tool): Item[] {
  return items.map((item) =>
    item.type === 'tool' ? { type: 'tool', tool: mapTool(item.tool, id, fn) } : item,
  );
}

function foldAgent(turn: LiveTurn, ev: AgentEvent): LiveTurn {
  const { items } = turn;
  switch (ev.kind) {
    case 'text': {
      const last = items.at(-1);
      if (last?.type === 'text') {
        return {
          ...turn,
          items: [...items.slice(0, -1), { type: 'text', text: last.text + (ev.text ?? '') }],
        };
      }
      return { ...turn, items: [...items, { type: 'text', text: ev.text ?? '' }] };
    }
    case 'tool_call': {
      const tool: Tool = {
        id: ev.toolId ?? '',
        name: ev.toolName ?? '',
        input: ev.input,
        done: false,
        output: '',
        isError: false,
        children: [],
      };
      const parent = ev.parentToolId;
      if (
        parent === undefined ||
        !items.some((i) => i.type === 'tool' && hasTool(i.tool, parent))
      ) {
        return { ...turn, items: [...items, { type: 'tool', tool }] };
      }
      return {
        ...turn,
        items: mapItems(items, parent, (p) => ({ ...p, children: [...p.children, tool] })),
      };
    }
    case 'tool_result':
      return {
        ...turn,
        items: mapItems(items, ev.toolId ?? '', (t) => ({
          ...t,
          done: true,
          output: ev.output ?? '',
          isError: ev.isError ?? false,
        })),
      };
    default:
      return turn;
  }
}

export function foldEvent(turn: LiveTurn, ev: StreamEvent): LiveTurn {
  if (ev.type === 'agent') return foldAgent(turn, ev.event);
  if (ev.status === 'idle') return emptyTurn;
  if (ev.status === 'running') return { status: 'running', items: [] };
  return { ...turn, status: ev.status, error: ev.error, note: ev.note };
}
